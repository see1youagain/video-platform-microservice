package video

import (
"context"
"io"
"strconv"

"video-platform-microservice/gateway/internal/logger"
"video-platform-microservice/gateway/internal/validator"
videoupload "video-platform-microservice/gateway/kitex_gen/videoupload"
"video-platform-microservice/gateway/rpc"

"github.com/cloudwego/hertz/pkg/app"
"github.com/cloudwego/hertz/pkg/protocol/consts"
"go.uber.org/zap"
)

// UploadChunkHandler 上传分片 → videoUpload 服务（multipart/form-data）
func UploadChunkHandler(ctx context.Context, c *app.RequestContext) {
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

fileHash := c.PostForm("file_hash")
indexStr := c.PostForm("index")
requestID := c.PostForm("request_id")

if fileHash == "" {
c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": "file_hash 不能为空"})
return
}
if err := validator.ValidateFileHash(fileHash); err != nil {
c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": err.Error()})
return
}

chunkIndex, err := strconv.Atoi(indexStr)
if err != nil {
c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": "index 必须是整数"})
return
}

// 读取分片数据
fileHeader, err := c.Request.FormFile("chunk")
if err != nil {
c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": "无法获取分片文件"})
return
}
fileReader, err := fileHeader.Open()
if err != nil {
c.JSON(consts.StatusInternalServerError, map[string]interface{}{"code": 500, "msg": "打开分片文件失败"})
return
}
defer fileReader.Close()

chunkData, err := io.ReadAll(fileReader)
if err != nil {
c.JSON(consts.StatusInternalServerError, map[string]interface{}{"code": 500, "msg": "读取分片失败"})
return
}

logger.Logger.Info("UploadChunk → videoUpload",
zap.Any("trace_id", traceID),
zap.String("file_hash", fileHash),
zap.Int("chunk_index", chunkIndex),
)

resp, err := rpc.VideoUploadClient.UploadChunk(ctx, &videoupload.UploadChunkReq{
FileHash:   fileHash,
ChunkIndex: int32(chunkIndex),
ChunkData:  chunkData,
UserId:     &userIDStr,
RequestId:  &requestID,
})
if err != nil {
logger.Logger.Error("VideoUpload UploadChunk 失败", zap.Any("trace_id", traceID), zap.Error(err))
c.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
"code":     503,
"msg":      "上传服务暂不可用",
"fallback": "simpleUpload",
})
return
}

c.JSON(consts.StatusOK, map[string]interface{}{
"code":             resp.Code,
"msg":              resp.Msg,
"already_uploaded": resp.GetAlreadyUploaded(),
})
}
