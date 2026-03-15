package video

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"

	"video-platform-microservice/gateway/internal/logger"
	videogen "video-platform-microservice/gateway/kitex_gen/videomanager"
	"video-platform-microservice/gateway/rpc"
)

func contextValueToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case uint:
		return strconv.FormatUint(uint64(val), 10)
	case uint32:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	default:
		return ""
	}
}

func bizCodeToHTTPStatus(code int32) int {
	switch code {
	case 200:
		return consts.StatusOK
	case 400:
		return consts.StatusBadRequest
	case 401:
		return consts.StatusUnauthorized
	case 403:
		return consts.StatusForbidden
	case 404:
		return consts.StatusNotFound
	case 409:
		return consts.StatusConflict
	default:
		return consts.StatusInternalServerError
	}
}

// DownloadHandler 下载视频 — 通过获取 MinIO 预签名 URL 后重定向。
// 原 DownloadChunk RPC 已废弃，视频存储迁移到 MinIO 直传后无需服务端中转。
func DownloadHandler(ctx context.Context, c *app.RequestContext) {
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
	traceIDStr := contextValueToString(traceID)
	userIDStr := contextValueToString(userID)

	logger.Logger.Info("获取下载链接",
		zap.String("trace_id", traceIDStr),
		zap.String("user_id", userIDStr),
		zap.String("file_hash", fileHash),
	)

	resp, err := rpc.VideoManagerClient.GetVideoInfo(ctx, &videogen.GetVideoInfoReq{
		FileHash: fileHash,
		UserId:   &userIDStr,
	})
	if err != nil {
		logger.Logger.Error("RPC GetVideoInfo 失败",
			zap.String("trace_id", traceIDStr),
			zap.Error(err),
		)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "服务器错误",
		})
		return
	}

	if resp.Code != 200 {
		c.JSON(bizCodeToHTTPStatus(resp.Code), map[string]interface{}{
			"code": resp.Code,
			"msg":  resp.Msg,
		})
		return
	}

	if resp.Url == nil || *resp.Url == "" {
		c.JSON(consts.StatusNotFound, map[string]interface{}{
			"code": 404,
			"msg":  "视频 URL 不可用",
		})
		return
	}

	c.Redirect(consts.StatusFound, []byte(*resp.Url))
}

// GetVideoInfoHandler 获取视频信息（需要认证）
func GetVideoInfoHandler(ctx context.Context, c *app.RequestContext) {
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
	traceIDStr := contextValueToString(traceID)
	userIDStr := contextValueToString(userID)

	logger.Logger.Info("获取视频信息",
		zap.String("trace_id", traceIDStr),
		zap.String("user_id", userIDStr),
		zap.String("file_hash", fileHash),
	)

	resp, err := rpc.VideoManagerClient.GetVideoInfo(ctx, &videogen.GetVideoInfoReq{
		FileHash: fileHash,
		UserId:   &userIDStr,
	})
	if err != nil {
		logger.Logger.Error("RPC 调用失败",
			zap.String("trace_id", traceIDStr),
			zap.Error(err),
		)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "服务器错误",
		})
		return
	}

	status := bizCodeToHTTPStatus(resp.Code)
	c.JSON(status, map[string]interface{}{
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
