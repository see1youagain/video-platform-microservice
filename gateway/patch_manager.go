package main

import (
"io/ioutil"
"strings"
)

func main() {
file := "/home/lzzy/project/go_project/video-platform-microservice/rpc-videoManager/handler.go"
content, _ := ioutil.ReadFile(file)
str := string(content)

replacement := `g managerDb.File
if err := commondb.GetDB().Where("file_hash = ? AND status = 'finished'", req.FileHash).First(&existing).Error; err == nil {
status := "finished"
resp.Code = 200
resp.Msg = "秒传成功"
resp.Status = &status
resp.Url = &existing.URL
return resp, nil
}

var uploadingFile managerDb.File
uploadingErr := commondb.GetDB().Where("file_hash = ? AND user_id = ?", req.FileHash, req.UserId).First(&uploadingFile).Error
if uploadingErr != nil {
reqId := ""
if req.RequestId != nil {
reqId = *req.RequestId
}
w := int32(0)
if req.Width != nil {
w = *req.Width
}
h := int32(0)
if req.Height != nil {
h = *req.Height
}
newFile := managerDb.File{
FileHash:  req.FileHash,
UserID:    req.UserId,
Filename:  req.Filename,
FileSize:  req.FileSize,
Status:    "uploading",
RequestID: reqId,
Width:     w,
Height:    h,
}
commondb.GetDB().Create(&newFile)
} else if uploadingFile.Status == "deleted" {
commondb.GetDB().Model(&uploadingFile).Update("status", "uploading")
}`

str = strings.Replace(str, `g managerDb.File
if err := commondb.GetDB().Where("file_hash = ? AND status = 'finished'", req.FileHash).First(&existing).Error; err == nil {
status := "finished"
resp.Code = 200
resp.Msg = "秒传成功"
resp.Status = &status
resp.Url = &existing.URL
return resp, nil
}`, replacement, 1)

ioutil.WriteFile(file, []byte(str), 0644)
}
