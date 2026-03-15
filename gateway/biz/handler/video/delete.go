package video

import (
	"context"
	"strconv"

	"video-platform-microservice/gateway/internal/logger"
	videomanager "video-platform-microservice/gateway/kitex_gen/videomanager"
	"video-platform-microservice/gateway/rpc"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

// DeleteVideoHandler 删除视频元数据（通过 videomanager 服务）
func DeleteVideoHandler(ctx context.Context, c *app.RequestContext) {
	var req struct {
		FileHash string `json:"file_hash" binding:"required"`
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
	if req.FileHash == "" {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": "file_hash 不能为空"})
		return
	}

	logger.Logger.Info("DeleteVideo → videoManager",
		zap.Any("trace_id", traceID),
		zap.String("file_hash", req.FileHash),
		zap.String("user_id", userIDStr),
	)

	resp, err := rpc.VideoManagerClient.DeleteVideo(ctx, &videomanager.DeleteVideoReq{
		FileHash: req.FileHash,
		UserId:   userIDStr,
	})
	if err != nil {
		logger.Logger.Error("VideoManager DeleteVideo 失败", zap.Any("trace_id", traceID), zap.Error(err))
		c.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
			"code": 503,
			"msg":  "视频管理服务暂不可用",
		})
		return
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"code": resp.Code,
		"msg":  resp.Msg,
	})
}
