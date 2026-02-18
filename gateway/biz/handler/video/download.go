package video

import (
"context"
"fmt"
"strconv"

"github.com/cloudwego/hertz/pkg/app"
"github.com/cloudwego/hertz/pkg/protocol/consts"
"go.uber.org/zap"

"video-platform-microservice/gateway/internal/logger"
videogen "video-platform-microservice/gateway/kitex_gen/video"
"video-platform-microservice/gateway/rpc"
)

// DownloadHandler 下载视频（支持Range，需要认证）
func DownloadHandler(ctx context.Context, c *app.RequestContext) {
// 从JWT context获取user_id
userID, exists := c.Get("user_id")
if !exists {
c.JSON(consts.StatusUnauthorized, map[string]interface{}{
"code": 401,
"msg":  "未授权，请先登录",
})
return
}

fileHash := c.Query("file_hash")
startStr := c.Query("start")
endStr := c.Query("end")

if fileHash == "" {
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  "file_hash 不能为空",
})
return
}

var startByte, endByte int64
if startStr != "" {
startByte, _ = strconv.ParseInt(startStr, 10, 64)
}
if endStr != "" {
endByte, _ = strconv.ParseInt(endStr, 10, 64)
}

traceID, _ := c.Get("trace_id")
logger.Logger.Info("下载视频",
zap.String("trace_id", traceID.(string)),
zap.String("user_id", userID.(string)),
zap.String("file_hash", fileHash),
zap.Int64("start", startByte),
zap.Int64("end", endByte),
)

// 调用 RPC 服务
resp, err := rpc.VideoClient.DownloadChunk(ctx, &videogen.DownloadChunkReq{
FileHash:  fileHash,
StartByte: startByte,
EndByte:   endByte,
})

if err != nil {
logger.Logger.Error("RPC 调用失败",
zap.String("trace_id", traceID.(string)),
zap.Error(err),
)
c.JSON(consts.StatusInternalServerError, map[string]interface{}{
"code": 500,
"msg":  "服务器错误",
})
return
}

if resp.Code != 200 {
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": resp.Code,
"msg":  resp.Msg,
})
return
}

// 设置响应头
c.Response.Header.Set("Content-Type", "video/mp4")
c.Response.Header.Set("Accept-Ranges", "bytes")
c.Response.Header.Set("Content-Length", fmt.Sprintf("%d", len(resp.Data)))
if startByte > 0 || endByte > 0 {
c.Response.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", startByte, startByte+int64(len(resp.Data))-1, resp.TotalSize))
c.Status(consts.StatusPartialContent)
}

c.Data(consts.StatusOK, "video/mp4", resp.Data)
}

// GetVideoInfoHandler 获取视频信息（需要认证）
func GetVideoInfoHandler(ctx context.Context, c *app.RequestContext) {
// 从JWT context获取user_id
userID, exists := c.Get("user_id")
if !exists {
c.JSON(consts.StatusUnauthorized, map[string]interface{}{
"code": 401,
"msg":  "未授权，请先登录",
})
return
}

fileHash := c.Query("file_hash")

if fileHash == "" {
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  "file_hash 不能为空",
})
return
}

traceID, _ := c.Get("trace_id")
logger.Logger.Info("获取视频信息",
zap.String("trace_id", traceID.(string)),
zap.String("user_id", userID.(string)),
zap.String("file_hash", fileHash),
)

// 调用 RPC 服务
resp, err := rpc.VideoClient.GetVideoInfo(ctx, &videogen.GetVideoInfoReq{
FileHash: fileHash,
UserId:   userID.(string),
})

if err != nil {
logger.Logger.Error("RPC 调用失败",
zap.String("trace_id", traceID.(string)),
zap.Error(err),
)
c.JSON(consts.StatusInternalServerError, map[string]interface{}{
"code": 500,
"msg":  "服务器错误",
})
return
}

c.JSON(consts.StatusOK, map[string]interface{}{
"code":             resp.Code,
"msg":              resp.Msg,
"file_hash":        resp.FileHash,
"filename":         resp.Filename,
"file_size":        resp.FileSize,
"width":            resp.Width,
"height":           resp.Height,
"url":              resp.Url,
"transcode_urls":   resp.TranscodeUrls,
"transcode_status": resp.TranscodeStatus,
})
}
