package devConfig

type DevConfigReq struct {
	Module   string `json:"module"`
	NoParse  bool   `json:"noParse"`
	Async    any    `json:"async"`
	RemoteIP bool   `json:"remoteIp"`
	Data     any    `json:"data"`
	Device   string `json:"device"`
}
