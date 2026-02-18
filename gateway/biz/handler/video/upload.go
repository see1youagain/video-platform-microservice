package video

import (
"context"
"io"

"video-platform-microservice/gateway/internal/logger"
"video-platform-microservice/gateway/internal/validator"
"video-platform-microservice/gateway/rpc"
video "video-platform-microservice/gateway/kitex_gen/video"

"github.com/cloudwego/hertz/pkg/app"
"github.com/cloudwego/hertz/pkg/protocol/consts"
"go.uber.org/zap"
)

// UploadChunkHandler 处理上传分片请求 (支持multipart/form-data)
func UploadChunkHandler(ctx context.Context, c *app.RequestContext) {
traceID, _ := c.Get("trace_id")

// 从multipart form获取参数
fileHash := c.PostForm("file_hash")
index := c.PostForm("index")

// 验证必填参数
if fileHash == "" {
logger.Logger.Warn("文件哈希不能为空",
zap.Any("trace_id", traceID),
)
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  "文件哈希不能为空",
})
return
}

if index == "" {
logger.Logger.Warn("分片索引不能为空",
zap.Any("trace_id", traceID),
)
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  "分片索引不能为空",
})
return
}

// 验证文件哈希格式
if err := validator.ValidateFileHash(fileHash); err != nil {
logger.Logger.Warn("文件哈希验证失败",
zap.Any("trace_id", traceID),
zap.String("file_hash", fileHash),
zap.Error(err),
)
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  err.Error(),
})
return
}

// 获取上传的文件
file, err := c.FormFile("chunk")
if err != nil {
logger.Logger.Warn("获取上传文件失败",
zap.Any("trace_id", traceID),
zap.Error(err),
)
c.JSON(consts.StatusBadRequest, map[string]interface{}{
"code": 400,
"msg":  "未找到上传文件",
})
return
}

// 打开文件读取数据
src, err := file.Open()
if err != nil {
logger.Logger.Error("打开上传文件失败",
zap.Any("trace_id", traceID),
zap.Error(err),
)
c.JSON(consts.StatusInternalServerError, map[string]interface{}{
"code": 500,
"msg":  "处理上传文件失败",
})
return
}
defer src.Close()

// 读取文件内容
data, err := io.ReadAll(src)
if err != nil {
logger.Logger.Error("读取文件数据失败",
zap.Any("trace_id", traceID),
zap.Error(err),
)
c.JSON(consts.StatusInternalServerError, map[string]interface{}{
"code": 500,
"msg":  "读取文件数据失败",
})
return
}

logger.Logger.Info("调用 RPC UploadChunk",
zap.Any("trace_id", traceID),
zap.String("file_hash", fileHash),
zap.String("index", index),
zap.Int("chunk_size", len(data)),
)

resp, err := rpc.VideoClient.UploadChunk(ctx, &video.UploadChunkReq{
FileHash: fileHash,
Index:    index,
Data:     data,
})

if err != nil {
logger.Logger.Error("RPC 调用失败",
zap.Any("trace_id", traceID),
zap.Error(err),
)
c.JSON(consts.StatusInternalServerError, map[string]interface{}{
"code": 500,
"msg":  "服务暂时不可用，请稍后重试",
})
return
}

var httpStatus int
switch resp.Code {
case 200:
httpStatus = consts.StatusOK
case 400:
httpStatus = consts.StatusBadRequest
default:
httpStatus = consts.StatusInternalServerError
}

c.JSON(httpStatus, map[string]interface{}{
"code": resp.Code,
"msg":  resp.Msg,
})
}
