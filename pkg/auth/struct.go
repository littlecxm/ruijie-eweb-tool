package auth

type LoginReq struct {
	Username             string `json:"username"`
	Time                 string `json:"time"`
	Encry                bool   `json:"encry"`
	Pwd                  string `json:"pwd"`
	IsCheckReadAgreement string `json:"isCheckReadAgreement"`
}
