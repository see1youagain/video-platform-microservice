namespace go videoupload

// ─── 初始化上传（秒传检测）────────────────────────
struct InitUploadReq {
    1: required string file_hash
    2: optional string filename
    3: optional i64    file_size
    4: optional string user_id
    5: optional i32    width
    6: optional i32    height
    7: optional string request_id
}

struct InitUploadResp {
    1: required i32          code
    2: required string       msg
    3: optional string       status           // "finished" | "partial" | "new"
    4: optional string       url              // 秒传时直接返回 URL
    5: optional list<string> finished_chunks
}

// ─── 上传分片 ──────────────────────────────────────
struct UploadChunkReq {
    1: required string file_hash
    2: required i32    chunk_index
    3: required binary chunk_data
    4: optional string user_id
    5: optional string request_id
}

struct UploadChunkResp {
    1: required i32    code
    2: required string msg
    3: optional bool   already_uploaded
}

// ─── 合并并发布（完成后发 FileUploaded Kafka 事件）─────
struct FinalizeUploadReq {
    1: required string       file_hash
    2: required string       filename
    3: required i32          total_chunks
    4: optional string       user_id
    5: optional i32          width
    6: optional i32          height
    7: optional string       request_id
    8: optional list<string> resolutions     // 需要的转码分辨率，透传给事件
}

struct FinalizeUploadResp {
    1: required i32    code
    2: required string msg
    3: optional string url        // MinIO 原片地址
    4: optional string task_id   // 占位，实际任务由 videoTranscode 自动创建
}

// ─── 简单上传（降级路径）──────────────────────────
struct SimpleUploadReq {
    1: required binary file_data
    2: required string filename
    3: required string file_hash
    4: optional string user_id
}

struct SimpleUploadResp {
    1: required i32    code
    2: required string msg
    3: optional string url
}

// ─── 服务定义 ─────────────────────────────────────
service VideoUploadService {
    InitUploadResp    InitUpload    (1: InitUploadReq    req)
    UploadChunkResp   UploadChunk   (1: UploadChunkReq   req)
    FinalizeUploadResp FinalizeUpload(1: FinalizeUploadReq req)
    SimpleUploadResp  SimpleUpload  (1: SimpleUploadReq  req)
}
