package video

import (
"context"
	"strconv"

"video-platform-microservice/gateway/internal/logger"
"video-platform-microservice/gateway/internal/validator"
videoupload "video-platform-microservice/gateway/kitex_gen/videoupload"
"video-platform-microservice/gateway/rpc"

"github.com/cloudwego/hertz/pkg/app"
"github.com/cloudwego/hertz/pkg/protocol/consts"
"go.uber.org/zap"
)

// InitUploadHandler 初始化分片上传 → 路由到 videoUpload 服务
func InitUploadHandler(ctx context.Context, c *app.RequestContext) {
var req struct {
FileHash  string `json:"file_hash"  binding:"required"`
Filename  string `json:"filename"   binding:"required"`
FileSize  int64  `json:"file_size"`
Width     int32  `json:"width"`
Height    int32  `json:"height"`
RequestID string `json:"request_id"`
}

traceID, _ := c.Get("trace_id")
userID, _ := c.Get("user_id")
var userIDStr string
switch v := userID.(type) {
case string:
userIDStr = v
case int64:
userIDStr = strconv.FormatInt(v, 10)
case uint:
userIDStr = strconv.FormatUint(uint64(v), 10)
}

if err := c.BindAndValidate(&req); err != nil {
c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": "参数错误: " + err.Error()})
return
}
if err := validator.ValidateFileHash(req.FileHash); err != nil {
c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": err.Error()})
return
}

logger.Logger.Info("InitUpload → videoUpload",
zap.Any("trace_id", traceID),
zap.String("file_hash", req.FileHash),
)

rpcReq := &videoupload.InitUploadReq{
FileHash:  req.FileHash,
Filename:  &req.Filename,
FileSize:  &req.FileSize,
UserId:    &userIDStr,
RequestId: &req.RequestID,
}
if req.Width != 0 {
rpcReq.Width = &req.Width
}
if req.Height != 0 {
rpcReq.Height = &req.Height
}

resp, err := rpc.VideoUploadClient.InitUpload(ctx, rpcReq)
if err != nil {
logger.Logger.Error("VideoUpload RPC 失败", zap.Any("trace_id", traceID), zap.Error(err))
c.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
"code":     503,
"msg":      "上传服务暂不可用",
"fallback": "simpleUpload",
})
return
}

c.JSON(consts.StatusOK, map[string]interface{}{
"code":            resp.Code,
"msg":             resp.Msg,
"status":          resp.GetStatus(),
"finished_chunks": resp.FinishedChunks,
"url":             resp.GetUrl(),
})
}
