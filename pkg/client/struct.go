package client

import "encoding/json"

type ApiCommonReq struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}

type ApiModuleReq struct {
	Module string `json:"module"`
	Device string `json:"device"`
}

type ApiReq struct {
	Method string `json:"method"`
	Params struct {
		Device string `json:"device"`
		Params []struct {
			ApiCommonReq
		} `json:"params"`
	} `json:"params"`
}

type ApiResp struct {
	Code    *int            `json:"code"`
	ID      *int            `json:"id"`
	Data    json.RawMessage `json:"data"`
	Error   json.RawMessage `json:"error"`
	Message string          `json:"message"`
}

type ApiErrResp struct {
	Code     int     `json:"code"`
	HttpCode *string `json:"http_code"`
	Msg      string  `json:"msg"`
	Message  string  `json:"message"`
}
