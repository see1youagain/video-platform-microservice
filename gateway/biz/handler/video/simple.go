package video

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"video-platform-microservice/gateway/internal/logger"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

// SimpleUploadHandler 处理简单上传请求（降级模式：单次全量上传）
func SimpleUploadHandler(ctx context.Context, c *app.RequestContext) {
	traceID, _ := c.Get("trace_id")

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"code": 400,
			"msg":  "缺少文件字段 file",
		})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "打开上传文件失败",
		})
		return
	}
	defer src.Close()

	baseDir := "/tmp/video-platform/simple"
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "创建存储目录失败",
		})
		return
	}

	filename := filepath.Base(file.Filename)
	targetName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filename)
	targetPath := filepath.Join(baseDir, targetName)

	dst, err := os.Create(targetPath)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "创建目标文件失败",
		})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "写入文件失败",
		})
		return
	}

	logger.Logger.Warn("simpleUpload 降级路径生效",
		zap.Any("trace_id", traceID),
		zap.String("target_path", targetPath),
	)

	c.JSON(consts.StatusOK, map[string]interface{}{
		"code": 200,
		"msg":  "simpleUpload 上传成功",
		"url":  fmt.Sprintf("/files/simple/%s", targetName),
	})
}
