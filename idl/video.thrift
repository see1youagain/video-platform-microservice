namespace go video

struct InitUploadReq {
    1: string file_hash
    2: string filename // 可选
    3: i64 file_size
    4: string user_id  // 用户ID，用于秒传判断
    5: i32 width       // 视频宽度（分辨率）
    6: i32 height      // 视频高度（分辨率）
    7: string request_id // 请求ID，用于幂等性
}

struct InitUploadResp {
    1: i32 code
    2: string msg
    3: string status // "uploading" or "finished"
    4: list<string> finished_chunks
    5: string url
}

struct UploadChunkReq {
    1: string file_hash
    2: string index
    3: binary data // 核心：通过 RPC 传输分片数据
    4: string user_id  // 用户ID
}

struct UploadChunkResp {
    1: i32 code
    2: string msg
}

struct MergeFileReq {
    1: string file_hash
    2: string filename
    3: i32 total_chunks
    4: string user_id
    5: i32 width       // 视频宽度
    6: i32 height      // 视频高度
    7: string request_id // 请求ID，用于幂等性
}

struct MergeFileResp {
    1: i32 code
    2: string msg
    3: string url
}

// 下载视频分片
struct DownloadChunkReq {
    1: string file_hash
    2: i32 chunk_index
    3: i64 start_byte    // 起始字节
    4: i64 end_byte      // 结束字节
}

struct DownloadChunkResp {
    1: i32 code
    2: string msg
    3: binary data
    4: i64 total_size
}

// 获取视频信息
struct GetVideoInfoReq {
    1: string file_hash
    2: string user_id
}

struct GetVideoInfoResp {
    1: i32 code
    2: string msg
    3: string file_hash
    4: string filename
    5: i64 file_size
    6: i32 width
    7: i32 height
    8: string url
    9: list<string> transcode_urls  // 转码后的URL列表
    10: string transcode_status     // "pending", "processing", "completed", "failed"
}

// 转码请求
struct TranscodeReq {
    1: string file_hash
    2: string user_id
    3: list<string> resolutions  // 例如 ["720p", "480p", "360p"]
    4: string request_id // 请求ID，用于幂等性
}

struct TranscodeResp {
    1: i32 code
    2: string msg
    3: string task_id
}

// 获取转码状态
struct GetTranscodeStatusReq {
    1: string task_id
}

struct GetTranscodeStatusResp {
    1: i32 code
    2: string msg
    3: string status
    4: i32 progress  // 0-100
    5: list<string> completed_urls
}

service VideoService {
    InitUploadResp InitUpload(1: InitUploadReq req)
    UploadChunkResp UploadChunk(1: UploadChunkReq req)
    MergeFileResp MergeFile(1: MergeFileReq req)
    DownloadChunkResp DownloadChunk(1: DownloadChunkReq req)
    GetVideoInfoResp GetVideoInfo(1: GetVideoInfoReq req)
    TranscodeResp Transcode(1: TranscodeReq req)
    GetTranscodeStatusResp GetTranscodeStatus(1: GetTranscodeStatusReq req)
}
