namespace go videoupload

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
    3: optional string       status
    4: optional string       url
    5: optional list<string> finished_chunks
    6: optional string       upload_id
}

struct UploadChunkReq {
    1: required string file_hash
    2: required i32    chunk_index
    3: required binary chunk_data
    4: optional string user_id
    5: optional string request_id
    6: optional string upload_id
}

struct UploadChunkResp {
    1: required i32    code
    2: required string msg
    3: optional bool   already_uploaded
}

struct FinalizeUploadReq {
    1: required string       file_hash
    2: required string       filename
    3: required i32          total_chunks
    4: optional string       user_id
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

struct QueryProgressReq {
    1: required string upload_id
}

struct QueryProgressResp {
    1: required i32 code
    2: required string msg
    3: optional i32 uploaded_parts
}

struct AbortUploadReq {
    2: required string file_hash
    1: required string upload_id
}

struct AbortUploadResp {
    1: required i32 code
    2: required string msg
}

service VideoUploadService {
    InitUploadResp    InitUpload    (1: InitUploadReq    req)
    UploadChunkResp   UploadChunk   (1: UploadChunkReq   req)
    FinalizeUploadResp FinalizeUpload(1: FinalizeUploadReq req)
    SimpleUploadResp  SimpleUpload  (1: SimpleUploadReq  req)
    QueryProgressResp QueryProgress (1: QueryProgressReq req)
    AbortUploadResp   AbortUpload   (1: AbortUploadReq   req)
}
