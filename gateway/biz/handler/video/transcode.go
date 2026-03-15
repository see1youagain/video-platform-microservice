package video

import (
"context"
"fmt"
"strconv"

"github.com/cloudwego/hertz/pkg/app"
"github.com/cloudwego/hertz/pkg/protocol/consts"
"go.uber.org/zap"

"video-platform-microservice/gateway/internal/logger"
videogen "video-platform-microservice/gateway/kitex_gen/videomanager"
"video-platform-microservice/gateway/rpc"
)

// TranscodeHandler 创建转码任务
func TranscodeHandler(ctx context.Context, c *app.RequestContext) {
var req struct {
FileHash    string   `json:"file_hash" binding:"required"`
Resolutions []string `json:"resolutions" binding:"required"`
}

if err := c.BindAndValidate(&req); err != nil {
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  fmt.Sprintf("参数错误: %v", err),
})
return
}

if len(req.Resolutions) == 0 {
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  "resolutions 不能为空",
})
return
}

traceIDRaw, _ := c.Get("trace_id")
traceID, _ := traceIDRaw.(string)
userIDRaw, _ := c.Get("user_id")
var userIDStr string
if v, ok := userIDRaw.(int64); ok {
userIDStr = strconv.FormatInt(v, 10)
} else if v, ok := userIDRaw.(string); ok {
userIDStr = v
}

logger.Logger.Info("创建转码任务",
zap.String("trace_id", traceID),
zap.String("file_hash", req.FileHash),
zap.String("user_id", userIDStr),
zap.Any("resolutions", req.Resolutions),
)

resp, err := rpc.VideoManagerClient.Transcode(ctx, &videogen.TranscodeReq{
FileHash:    req.FileHash,
UserId:      userIDStr,
Resolutions: req.Resolutions,
})
if err != nil {
logger.Logger.Error("RPC 调用失败",
zap.String("trace_id", traceID),
zap.Error(err),
)
c.JSON(consts.StatusInternalServerError, map[string]interface{}{
"code": 500,
"msg":  "服务器错误",
})
return
}

c.JSON(consts.StatusOK, map[string]interface{}{
"code":    resp.Code,
"msg":     resp.Msg,
"task_id": resp.TaskId,
})
}

// GetTranscodeStatusHandler 获取转码状态
func GetTranscodeStatusHandler(ctx context.Context, c *app.RequestContext) {
taskID := c.Query("task_id")
if taskID == "" {
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  "task_id 不能为空",
})
return
}

traceIDRaw, _ := c.Get("trace_id")
traceID, _ := traceIDRaw.(string)
logger.Logger.Info("查询转码状态",
zap.String("trace_id", traceID),
zap.String("task_id", taskID),
)

resp, err := rpc.VideoManagerClient.GetTranscodeStatus(ctx, &videogen.GetTranscodeStatusReq{
TaskId: taskID,
})
if err != nil {
logger.Logger.Error("RPC 调用失败",
zap.String("trace_id", traceID),
zap.Error(err),
)
c.JSON(consts.StatusInternalServerError, map[string]interface{}{
"code": 500,
"msg":  "服务器错误",
})
return
}

c.JSON(consts.StatusOK, map[string]interface{}{
"code":           resp.Code,
"msg":            resp.Msg,
"status":         resp.Status,
"progress":       resp.Progress,
"completed_urls": resp.CompletedUrls,
})
}
