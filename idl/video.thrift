namespace go videomanager

// ─── 上传协调（委托 videoUpload，通过 RPC 中转）────────────────────────────
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
    3: optional string        status
    4: optional list<string>  finished_chunks
    5: optional string        url
    6: optional string        upload_id
}

struct FinalizeUploadReq {
    1: required string       file_hash
    2: required string       filename
    3: required i32          total_chunks
    4: required string       user_id
    5: optional i32          width
    6: optional i32          height
    7: optional string       request_id
    8: optional list<string> resolutions
    9: optional string       upload_id
}

struct FinalizeUploadResp {
    1: required i32    code
    2: required string msg
    3: optional string url
    4: optional string task_id
}

// ─── 进度查询与中止（委托 videoUpload RPC）──────────────────────────────────
struct QueryUploadProgressReq {
    1: required string upload_id
}

struct QueryUploadProgressResp {
    1: required i32    code
    2: required string msg
    3: optional i32    uploaded_parts
    4: optional i32    total_parts
}

struct AbortUploadReq {
    1: required string upload_id
    2: required string file_hash
}

struct AbortUploadResp {
    1: required i32    code
    2: required string msg
}

// ─── 视频信息管理 ─────────────────────────────────────────────────────────
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
    10: optional string       transcode_status
}

struct DeleteVideoReq {
    1: required string file_hash
    2: required string user_id
}

struct DeleteVideoResp {
    1: required i32    code
    2: required string msg
}

// ─── 转码调度（使用 Outbox 保证双写一致性）──────────────────────────────────
struct TranscodeReq {
    1: required string       file_hash
    2: required string       user_id
    3: required list<string> resolutions
    4: optional string       request_id
}

struct TranscodeResp {
    1: required i32    code
    2: required string msg
    3: optional string task_id
}

struct GetTranscodeStatusReq {
    1: required string task_id
}

struct GetTranscodeStatusResp {
    1: required i32           code
    2: required string        msg
    3: optional string        status
    4: optional double        progress
    5: optional list<string>  completed_urls
}

// ─── 服务定义 ─────────────────────────────────────────────────────────────
service VideoManagerService {
    InitUploadResp          InitUpload          (1: InitUploadReq          req)
    FinalizeUploadResp      FinalizeUpload       (1: FinalizeUploadReq      req)
    QueryUploadProgressResp QueryUploadProgress  (1: QueryUploadProgressReq req)
    AbortUploadResp         AbortUpload          (1: AbortUploadReq         req)
    GetVideoInfoResp        GetVideoInfo         (1: GetVideoInfoReq        req)
    DeleteVideoResp         DeleteVideo          (1: DeleteVideoReq         req)
    TranscodeResp           Transcode            (1: TranscodeReq           req)
    GetTranscodeStatusResp  GetTranscodeStatus   (1: GetTranscodeStatusReq req)
}
