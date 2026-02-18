package main

import (
"context"
	"github.com/bytedance/gopkg/cloud/metainfo"
"encoding/json"
"fmt"
"log"

"video-platform-microservice/rpc-video/internal/db"
"video-platform-microservice/rpc-video/internal/redis"
"video-platform-microservice/rpc-video/internal/storage"
"video-platform-microservice/rpc-video/internal/transcode"
video "video-platform-microservice/rpc-video/kitex_gen/video"

commonRedis "github.com/redis/go-redis/v9"
)

// VideoServiceImpl implements the last service interface defined in the IDL.
type VideoServiceImpl struct{}

// getUserIDFromContext 从 metainfo 中获取 user_id，如果不存在则使用请求中的 UserId
func getUserIDFromContext(ctx context.Context, reqUserID string) string {
// 优先从 metainfo 获取（网关传递的认证用户）
if userID, ok := metainfo.GetPersistentValue(ctx, "user_id"); ok {
log.Printf("[MetaInfo] 从 metainfo 获取 user_id: %s", userID)
return userID
}

// 其次使用请求中的 UserId（向后兼容）
if reqUserID != "" {
log.Printf("[Request] 使用请求参数中的 user_id: %s", reqUserID)
return reqUserID
}

// 默认使用 anonymous
log.Printf("[Default] 使用默认 user_id: anonymous")
return "anonymous"
}


// InitUpload 初始化上传（秒传检查）
func (s *VideoServiceImpl) InitUpload(ctx context.Context, req *video.InitUploadReq) (resp *video.InitUploadResp, err error) {
resp = &video.InitUploadResp{}

// 检查参数
if req.FileHash == "" {
resp.Code = 400
resp.Msg = "file_hash 不能为空"
return resp, nil
}

userID := req.UserId
if userID == "" {
userID = "anonymous"
}

log.Printf("[InitUpload] FileHash: %s, Filename: %s, UserID: %s", req.FileHash, req.Filename, userID)
// 0. 幂等性检查：如果request_id已存在，直接返回之前的结果
if req.RequestId != "" {
existingFile, err := db.CheckFileByRequestID(req.RequestId)
if err != nil {
log.Printf("[InitUpload] 幂等性检查失败: %v", err)
} else if existingFile != nil {
log.Printf("[InitUpload] 幂等性命中，RequestID: %s", req.RequestId)
resp.Code = 200
resp.Msg = "文件已存在（幂等性）"
resp.Status = existingFile.Status
resp.Url = existingFile.URL
return resp, nil
}
}


// 1. 检查墓碑（用户+文件哈希完全匹配的秒传）
cacheKey := fmt.Sprintf("tombstone:%s:%s", userID, req.FileHash)
url, err := redis.GetTombstone(ctx, cacheKey)
if err == nil {
log.Printf("[InitUpload] 秒传命中(用户级): %s", req.FileHash)
resp.Code = 200
resp.Msg = "文件已存在（秒传）"
resp.Status = "finished"
resp.Url = url
return resp, nil
}

// 2. 检查数据库（用户+文件哈希）
exists, fileURL, err := db.FileExistsByHashAndUser(req.FileHash, userID)
if err != nil {
log.Printf("[InitUpload] 数据库查询失败: %v", err)
resp.Code = 500
resp.Msg = "数据库查询失败"
return resp, nil
}

if exists {
log.Printf("[InitUpload] 数据库秒传命中(用户级): %s", req.FileHash)
redis.SetTombstone(ctx, cacheKey, fileURL)
resp.Code = 200
resp.Msg = "文件已存在（秒传）"
resp.Status = "finished"
resp.Url = fileURL
return resp, nil
}

// 3. 检查 Redis 上传状态
statusKey := fmt.Sprintf("upload:%s:%s", userID, req.FileHash)
status, err := redis.GetUploadStatus(ctx, statusKey)
if err != nil && err != commonRedis.Nil {
log.Printf("[InitUpload] Redis 查询失败: %v", err)
}

if status == "uploading" {
// 断点续传：返回已上传的分片列表
finishedChunks, _ := redis.GetFinishedChunks(ctx, statusKey)
log.Printf("[InitUpload] 断点续传，已完成 %d 个分片", len(finishedChunks))
resp.Code = 200
resp.Msg = "断点续传"
resp.Status = "uploading"
resp.FinishedChunks = finishedChunks
return resp, nil
}

// 4. 首次上传：设置状态
if err := redis.SetUploadStatus(ctx, statusKey, "uploading"); err != nil {
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

userID := req.UserId
if userID == "" {
userID = "anonymous"
}

log.Printf("[UploadChunk] FileHash: %s, Index: %s, Size: %d bytes, UserID: %s", 
req.FileHash, req.Index, len(req.Data), userID)

// 1. 检查分片是否已存在（去重）
chunkKey := fmt.Sprintf("%s_%s_%s", userID, req.FileHash, req.Index)
if storage.ChunkExists(chunkKey, req.Index) {
log.Printf("[UploadChunk] 分片已存在，跳过: %s", chunkKey)
resp.Code = 200
resp.Msg = "分片已存在"
return resp, nil
}

// 2. 保存分片到磁盘
if err := storage.SaveChunk(chunkKey, req.Index, req.Data); err != nil {
log.Printf("[UploadChunk] 保存分片失败: %v", err)
resp.Code = 500
resp.Msg = fmt.Sprintf("保存分片失败: %v", err)
return resp, nil
}

// 3. 记录已完成的分片到 Redis
statusKey := fmt.Sprintf("upload:%s:%s", userID, req.FileHash)
if err := redis.AddFinishedChunk(ctx, statusKey, req.Index); err != nil {
log.Printf("[UploadChunk] Redis 记录失败: %v", err)
// 不影响主流程，继续
}

log.Printf("[UploadChunk] 分片上传成功: %s", chunkKey)
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

userID := req.UserId
if userID == "" {
userID = "anonymous"
}

log.Printf("[MergeFile] FileHash: %s, Filename: %s, TotalChunks: %d, UserID: %s", 
req.FileHash, req.Filename, req.TotalChunks, userID)

// 1. 检查所有分片是否已上传
statusKey := fmt.Sprintf("upload:%s:%s", userID, req.FileHash)
finishedChunks, err := redis.GetFinishedChunks(ctx, statusKey)
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
chunkPrefix := fmt.Sprintf("%s_%s", userID, req.FileHash)
if err := storage.MergeChunks(chunkPrefix, req.Filename, int(req.TotalChunks)); err != nil {
log.Printf("[MergeFile] 合并失败: %v", err)
resp.Code = 500
resp.Msg = fmt.Sprintf("合并失败: %v", err)
return resp, nil
}

// 3. 提取视频分辨率（如果有ffprobe）
filePath := storage.GetFilePath(chunkPrefix, req.Filename)
width, height, err := transcode.ExtractResolution(filePath)
if err != nil {
log.Printf("[MergeFile] 提取分辨率失败: %v", err)
// 使用传入的分辨率或默认值
width = req.Width
height = req.Height
}

// 4. 生成文件 URL
fileURL := storage.GetFileURL(chunkPrefix, req.Filename)

// 5. 更新数据库
fileSize, _ := storage.GetFileSize(chunkPrefix, req.Filename)
if err := db.CreateFileWithMetadata(req.FileHash, userID, req.Filename, fileSize, fileURL, width, height, req.RequestId); err != nil {
log.Printf("[MergeFile] 数据库创建失败: %v", err)
}

// 6. 设置墓碑（永久缓存）
cacheKey := fmt.Sprintf("tombstone:%s:%s", userID, req.FileHash)
if err := redis.SetTombstone(ctx, cacheKey, fileURL); err != nil {
log.Printf("[MergeFile] 设置墓碑失败: %v", err)
}

// 7. 清理上传缓存
if err := redis.DeleteUploadCache(ctx, statusKey); err != nil {
log.Printf("[MergeFile] 清理缓存失败: %v", err)
}

log.Printf("[MergeFile] 文件合并成功: %s -> %s (分辨率: %dx%d)", req.FileHash, fileURL, width, height)
resp.Code = 200
resp.Msg = "文件合并成功"
resp.Url = fileURL
return resp, nil
}

// DownloadChunk 下载文件分片（支持Range请求）
func (s *VideoServiceImpl) DownloadChunk(ctx context.Context, req *video.DownloadChunkReq) (resp *video.DownloadChunkResp, err error) {
resp = &video.DownloadChunkResp{}

if req.FileHash == "" {
resp.Code = 400
resp.Msg = "file_hash 不能为空"
return resp, nil
}

log.Printf("[DownloadChunk] FileHash: %s, Range: %d-%d", req.FileHash, req.StartByte, req.EndByte)

// 获取文件信息
file, err := db.GetFileByHash(req.FileHash)
if err != nil || file == nil {
resp.Code = 404
resp.Msg = "文件不存在"
return resp, nil
}

// 读取文件分片
chunkPrefix := fmt.Sprintf("%s_%s", file.UserID, req.FileHash)
data, totalSize, err := storage.ReadFileChunk(chunkPrefix, file.Filename, req.StartByte, req.EndByte)
if err != nil {
log.Printf("[DownloadChunk] 读取失败: %v", err)
resp.Code = 500
resp.Msg = fmt.Sprintf("读取失败: %v", err)
return resp, nil
}

resp.Code = 200
resp.Msg = "读取成功"
resp.Data = data
resp.TotalSize = totalSize

log.Printf("[DownloadChunk] 读取成功: %d bytes", len(data))
return resp, nil
}

// GetVideoInfo 获取视频信息
func (s *VideoServiceImpl) GetVideoInfo(ctx context.Context, req *video.GetVideoInfoReq) (resp *video.GetVideoInfoResp, err error) {
resp = &video.GetVideoInfoResp{}

if req.FileHash == "" {
resp.Code = 400
resp.Msg = "file_hash 不能为空"
return resp, nil
}

userID := req.UserId
if userID == "" {
userID = "anonymous"
}

log.Printf("[GetVideoInfo] FileHash: %s, UserID: %s", req.FileHash, userID)

// 查询数据库
file, err := db.GetFileByHashAndUser(req.FileHash, userID)
if err != nil {
resp.Code = 500
resp.Msg = "数据库查询失败"
return resp, nil
}

if file == nil {
resp.Code = 404
resp.Msg = "文件不存在"
return resp, nil
}

// 解析转码URL
var transcodeURLs []string
if file.TranscodeURLs != "" {
json.Unmarshal([]byte(file.TranscodeURLs), &transcodeURLs)
}

resp.Code = 200
resp.Msg = "查询成功"
resp.FileHash = file.FileHash
resp.Filename = file.Filename
resp.FileSize = file.FileSize
resp.Width = file.Width
resp.Height = file.Height
resp.Url = file.URL
resp.TranscodeUrls = transcodeURLs
resp.TranscodeStatus = file.TranscodeStatus

return resp, nil
}

// Transcode 创建转码任务
func (s *VideoServiceImpl) Transcode(ctx context.Context, req *video.TranscodeReq) (resp *video.TranscodeResp, err error) {
resp = &video.TranscodeResp{}

if req.FileHash == "" || len(req.Resolutions) == 0 {
resp.Code = 400
resp.Msg = "参数不完整"
return resp, nil
}

userID := req.UserId
if userID == "" {
userID = "anonymous"
}

log.Printf("[Transcode] FileHash: %s, UserID: %s, Resolutions: %v", req.FileHash, userID, req.Resolutions)
// 0. 幂等性检查：如果request_id已存在，直接返回之前的结果
if req.RequestId != "" {
existingTask, err := db.CheckTranscodeTaskByRequestID(req.RequestId)
if err != nil {
log.Printf("[Transcode] 幂等性检查失败: %v", err)
} else if existingTask != nil {
log.Printf("[Transcode] 幂等性命中，RequestID: %s, TaskID: %s", req.RequestId, existingTask.TaskID)
resp.Code = 200
resp.Msg = "转码任务已创建（幂等性）"
resp.TaskId = existingTask.TaskID
return resp, nil
}
}


// 检查文件是否存在
file, err := db.GetFileByHashAndUser(req.FileHash, userID)
if err != nil || file == nil {
resp.Code = 404
resp.Msg = "文件不存在"
return resp, nil
}

// 创建转码任务
taskID, err := transcode.CreateTask(req.FileHash, userID, req.Resolutions)
if err != nil {
log.Printf("[Transcode] 创建任务失败: %v", err)
resp.Code = 500
resp.Msg = fmt.Sprintf("创建任务失败: %v", err)
return resp, nil
}

resp.Code = 200
resp.Msg = "转码任务已创建"
resp.TaskId = taskID

log.Printf("[Transcode] 任务创建成功: %s", taskID)
return resp, nil
}

// GetTranscodeStatus 获取转码状态
func (s *VideoServiceImpl) GetTranscodeStatus(ctx context.Context, req *video.GetTranscodeStatusReq) (resp *video.GetTranscodeStatusResp, err error) {
resp = &video.GetTranscodeStatusResp{}

if req.TaskId == "" {
resp.Code = 400
resp.Msg = "task_id 不能为空"
return resp, nil
}

log.Printf("[GetTranscodeStatus] TaskID: %s", req.TaskId)

// 获取任务状态
status, err := transcode.GetTaskStatus(req.TaskId)
if err != nil {
log.Printf("[GetTranscodeStatus] 查询失败: %v", err)
resp.Code = 404
resp.Msg = "任务不存在"
return resp, nil
}

resp.Code = 200
resp.Msg = "查询成功"
resp.Status = status.Status
resp.Progress = status.Progress
resp.CompletedUrls = status.CompletedURLs

return resp, nil
}
