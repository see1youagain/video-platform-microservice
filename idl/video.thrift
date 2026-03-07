namespace go video

// ============================================================
// 初始化上传
// ============================================================
struct InitUploadReq {
    1: required string file_hash
    2: required string filename
    3: required i64    file_size
    4: required string user_id
    5: optional i32    width
    6: optional i32    height
    7: optional string request_id
}

struct InitUploadResp {
    1: required i32           code
    2: required string        msg
    3: optional string        status          // "new" | "exists" | "partial"
    4: optional list<string>  finished_chunks // 已上传的分片索引列表
    5: optional string        url             // 若已存在则直接返回访问 URL
}

// ============================================================
// 上传分片
// ============================================================
struct UploadChunkReq {
    1: required string file_hash
    2: required i32    index
    3: required binary data
    4: required string user_id
}

struct UploadChunkResp {
    1: required i32    code
    2: required string msg
}

// ============================================================
// 合并文件
// ============================================================
struct MergeFileReq {
    1: required string file_hash
    2: required string filename
    3: required i32    total_chunks
    4: required string user_id
    5: optional i32    width
    6: optional i32    height
    7: optional string request_id
}

struct MergeFileResp {
    1: required i32    code
    2: required string msg
    3: optional string url
}

// ============================================================
// 下载分片
// ============================================================
struct DownloadChunkReq {
    1: required string file_hash
    2: required i32    chunk_index
    3: optional i64    start_byte
    4: optional i64    end_byte
}

struct DownloadChunkResp {
    1: required i32    code
    2: required string msg
    3: optional binary data
    4: optional i64    total_size
}

// ============================================================
// 获取视频信息
// ============================================================
struct GetVideoInfoReq {
    1: required string file_hash
    2: optional string user_id
}

struct GetVideoInfoResp {
    1: required i32           code
    2: required string        msg
    3: optional string        file_hash
    4: optional string        filename
    5: optional i64           file_size
    6: optional i32           width
    7: optional i32           height
    8: optional string        url
    9: optional list<string>  transcode_urls
    10: optional string       transcode_status  // "pending" | "processing" | "done" | "failed"
}

// ============================================================
// 创建转码任务
// ============================================================
struct TranscodeReq {
    1: required string       file_hash
    2: required string       user_id
    3: required list<string> resolutions  // e.g. ["360p","720p","1080p"]
    4: optional string       request_id
}

struct TranscodeResp {
    1: required i32    code
    2: required string msg
    3: optional string task_id
}

// ============================================================
// 查询转码状态
// ============================================================
struct GetTranscodeStatusReq {
    1: required string task_id
}

struct GetTranscodeStatusResp {
    1: required i32           code
    2: required string        msg
    3: optional string        status         // "pending" | "processing" | "done" | "failed"
    4: optional double        progress       // 0.0 ~ 1.0
    5: optional list<string>  completed_urls
}

// ============================================================
// 服务定义
// ============================================================
service VideoService {
    InitUploadResp         InitUpload         (1: InitUploadReq         req)
    UploadChunkResp        UploadChunk        (1: UploadChunkReq        req)
    MergeFileResp          MergeFile          (1: MergeFileReq          req)
    DownloadChunkResp      DownloadChunk      (1: DownloadChunkReq      req)
    GetVideoInfoResp       GetVideoInfo       (1: GetVideoInfoReq       req)
    TranscodeResp          Transcode          (1: TranscodeReq          req)
    GetTranscodeStatusResp GetTranscodeStatus (1: GetTranscodeStatusReq req)
}
