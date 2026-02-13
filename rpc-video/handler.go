package main

import (
	"context"
	"fmt"
	"log"

	"video-platform-microservice/rpc-video/internal/db"
	"video-platform-microservice/rpc-video/internal/redis"
	"video-platform-microservice/rpc-video/internal/storage"
	video "video-platform-microservice/rpc-video/kitex_gen/video"

	redisLib "github.com/redis/go-redis/v9"
)

// VideoServiceImpl implements the last service interface defined in the IDL.
type VideoServiceImpl struct{}

// InitUpload 初始化上传（秒传检查）
func (s *VideoServiceImpl) InitUpload(ctx context.Context, req *video.InitUploadReq) (resp *video.InitUploadResp, err error) {
    resp = &video.InitUploadResp{}

    // 检查参数
    if req.FileHash == "" {
        resp.Code = 400
        resp.Msg = "file_hash 不能为空"
        return resp, nil
    }

    log.Printf("[InitUpload] FileHash: %s, Filename: %s", req.FileHash, req.Filename)

    // 1. 检查墓碑（文件是否已完全上传）
    url, err := redis.GetTombstone(ctx, req.FileHash)
    if err == nil {
        log.Printf("[InitUpload] 秒传命中: %s", req.FileHash)
        resp.Code = 200
        resp.Msg = "文件已存在（秒传）"
        resp.Status = "finished"
        resp.Url = url
        return resp, nil
    }

    // 2. 检查数据库是否有记录
    exists, fileURL, err := db.FileExistsByHash(req.FileHash)
    if err != nil {
        log.Printf("[InitUpload] 数据库查询失败: %v", err)
        resp.Code = 500
        resp.Msg = "数据库查询失败"
        return resp, nil
    }

    if exists {
        // 文件已完成上传
        log.Printf("[InitUpload] 数据库秒传命中: %s", req.FileHash)
        // 设置墓碑缓存
        redis.SetTombstone(ctx, req.FileHash, fileURL)
        resp.Code = 200
        resp.Msg = "文件已存在（秒传）"
        resp.Status = "finished"
        resp.Url = fileURL
        return resp, nil
    }

    // 3. 检查 Redis 上传状态
    status, err := redis.GetUploadStatus(ctx, req.FileHash)
    if err != nil && err != redisLib.Nil {
        log.Printf("[InitUpload] Redis 查询失败: %v", err)
    }

    if status == "uploading" {
        // 断点续传：返回已上传的分片列表
        finishedChunks, _ := redis.GetFinishedChunks(ctx, req.FileHash)
        log.Printf("[InitUpload] 断点续传，已完成 %d 个分片", len(finishedChunks))
        resp.Code = 200
        resp.Msg = "断点续传"
        resp.Status = "uploading"
        resp.FinishedChunks = finishedChunks
        return resp, nil
    }

    // 4. 首次上传：设置状态
    if err := redis.SetUploadStatus(ctx, req.FileHash, "uploading"); err != nil {
        log.Printf("[InitUpload] 设置上传状态失败: %v", err)
        resp.Code = 500
        resp.Msg = "初始化上传失败"
        return resp, nil
    }

    log.Printf("[InitUpload] 首次上传初始化成功")
    resp.Code = 200
    resp.Msg = "初始化成功"
    resp.Status = "uploading"
    resp.FinishedChunks = []string{}
    return resp, nil
}

// UploadChunk 上传分片
func (s *VideoServiceImpl) UploadChunk(ctx context.Context, req *video.UploadChunkReq) (resp *video.UploadChunkResp, err error) {
    resp = &video.UploadChunkResp{}

    // 检查参数
    if req.FileHash == "" || req.Index == "" || len(req.Data) == 0 {
        resp.Code = 400
        resp.Msg = "参数不完整"
        return resp, nil
    }

    log.Printf("[UploadChunk] FileHash: %s, Index: %s, Size: %d bytes", req.FileHash, req.Index, len(req.Data))

    // 1. 检查分片是否已存在（去重）
    if storage.ChunkExists(req.FileHash, req.Index) {
        log.Printf("[UploadChunk] 分片已存在，跳过: %s_%s", req.FileHash, req.Index)
        resp.Code = 200
        resp.Msg = "分片已存在"
        return resp, nil
    }

    // 2. 保存分片到磁盘
    if err := storage.SaveChunk(req.FileHash, req.Index, req.Data); err != nil {
        log.Printf("[UploadChunk] 保存分片失败: %v", err)
        resp.Code = 500
        resp.Msg = fmt.Sprintf("保存分片失败: %v", err)
        return resp, nil
    }

    // 3. 记录已完成的分片到 Redis
    if err := redis.AddFinishedChunk(ctx, req.FileHash, req.Index); err != nil {
        log.Printf("[UploadChunk] Redis 记录失败: %v", err)
        // 不影响主流程，继续
    }

    log.Printf("[UploadChunk] 分片上传成功: %s_%s", req.FileHash, req.Index)
    resp.Code = 200
    resp.Msg = "分片上传成功"
    return resp, nil
}

// MergeFile 合并文件
func (s *VideoServiceImpl) MergeFile(ctx context.Context, req *video.MergeFileReq) (resp *video.MergeFileResp, err error) {
    resp = &video.MergeFileResp{}

    // 检查参数
    if req.FileHash == "" || req.Filename == "" || req.TotalChunks <= 0 {
        resp.Code = 400
        resp.Msg = "参数不完整"
        return resp, nil
    }

    log.Printf("[MergeFile] FileHash: %s, Filename: %s, TotalChunks: %d", req.FileHash, req.Filename, req.TotalChunks)

    // 1. 检查所有分片是否已上传
    finishedChunks, err := redis.GetFinishedChunks(ctx, req.FileHash)
    if err != nil {
        log.Printf("[MergeFile] 获取已完成分片失败: %v", err)
        resp.Code = 500
        resp.Msg = "获取上传状态失败"
        return resp, nil
    }

    if int32(len(finishedChunks)) < req.TotalChunks {
        log.Printf("[MergeFile] 分片未上传完整: %d/%d", len(finishedChunks), req.TotalChunks)
        resp.Code = 400
        resp.Msg = fmt.Sprintf("分片未上传完整: %d/%d", len(finishedChunks), req.TotalChunks)
        return resp, nil
    }

    // 2. 合并分片
    if err := storage.MergeChunks(req.FileHash, req.Filename, int(req.TotalChunks)); err != nil {
        log.Printf("[MergeFile] 合并失败: %v", err)
        resp.Code = 500
        resp.Msg = fmt.Sprintf("合并失败: %v", err)
        return resp, nil
    }

    // 3. 生成文件 URL
    fileURL := storage.GetFileURL(req.FileHash, req.Filename)

    // 4. 更新数据库
    if err := db.UpdateFileStatus(req.FileHash, "finished", fileURL); err != nil {
        // 如果记录不存在，创建新记录
        if err := db.CreateFile(req.FileHash, req.Filename, 0, fileURL); err != nil {
            log.Printf("[MergeFile] 数据库更新失败: %v", err)
        }
    }

    // 5. 设置墓碑（永久缓存）
    if err := redis.SetTombstone(ctx, req.FileHash, fileURL); err != nil {
        log.Printf("[MergeFile] 设置墓碑失败: %v", err)
    }

    // 6. 清理上传缓存
    if err := redis.DeleteUploadCache(ctx, req.FileHash); err != nil {
        log.Printf("[MergeFile] 清理缓存失败: %v", err)
    }

    log.Printf("[MergeFile] 文件合并成功: %s -> %s", req.FileHash, fileURL)
    resp.Code = 200
    resp.Msg = "文件合并成功"
    resp.Url = fileURL
    return resp, nil
}