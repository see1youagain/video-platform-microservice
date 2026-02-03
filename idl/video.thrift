namespace go video

struct InitUploadReq {
    1: string file_hash
    2: string filename // 可选
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
}

struct UploadChunkResp {
    1: i32 code
    2: string msg
}

struct MergeFileReq {
    1: string file_hash
    2: string filename
    3: i32 total_chunks
}

struct MergeFileResp {
    1: i32 code
    2: string msg
    3: string url
}

service VideoService {
    InitUploadResp InitUpload(1: InitUploadReq req)
    UploadChunkResp UploadChunk(1: UploadChunkReq req)
    MergeFileResp MergeFile(1: MergeFileReq req)
}