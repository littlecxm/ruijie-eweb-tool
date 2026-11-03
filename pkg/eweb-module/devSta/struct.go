package devSta

import (
	"bytes"
	"encoding/json"
	"errors"
)

type DevStaReq struct {
	Module   string `json:"module"`
	NoParse  bool   `json:"noParse"`
	Async    any    `json:"async"`
	RemoteIP bool   `json:"remoteIp"`
	Data     any    `json:"data,omitempty"`
	Device   string `json:"device"`
}

// Value handles terminal fields that firmware encodes as either text or numbers.
type Value string

func (v *Value) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*v = ""
		return nil
	}
	var s string
	if json.Unmarshal(data, &s) == nil {
		*v = Value(s)
		return nil
	}
	var n json.Number
	if json.Unmarshal(data, &n) != nil {
		return errors.New("expected text or number")
	}
	*v = Value(n.String())
	return nil
}

type Terminal struct {
	MAC             string `json:"mac"`
	UserIP          string `json:"userIp"`
	Name            string `json:"name"`
	HostName        string `json:"hostName"`
	ConnectType     string `json:"connectType"`
	SN              string `json:"sn"`
	DeviceAliasName string `json:"deviceAliasName"`
	SSID            string `json:"ssid"`
	Band            string `json:"band"`
	Port            Value  `json:"port"`
	OnlineTime      Value  `json:"onlinetime"`
	RSSI            Value  `json:"rssi"`
	HardwareType    string `json:"hardwareType"`
	OSType          string `json:"osType"`
	Manufacture     string `json:"manufacture"`
}

type UserList struct {
	Total int        `json:"total"`
	List  []Terminal `json:"list"`
}
