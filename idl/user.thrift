namespace go user

struct RegisterReq {
    1: string username
    2: string password
}

struct RegisterResp {
    1: i32 code
    2: string msg
    3: i64 user_id
}

struct LoginReq {
    1: string username
    2: string password
}

struct LoginResp {
    1: i32 code
    2: string msg
    3: string token // JWT 在网关生成，但这里可以是 UserID 让网关生成
    4: i64 user_id
}

service UserService {
    RegisterResp Register(1: RegisterReq req)
    LoginResp Login(1: LoginReq req)
}