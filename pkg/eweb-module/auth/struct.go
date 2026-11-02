package auth

import "time"

type LoginData struct {
	Token    string `json:"token"`
	SN       string `json:"sn"`
	SID      string `json:"sid"`
	Reload   bool   `json:"reload"`
	LockTime int    `json:"lockTime"`
	Count    int    `json:"count"`
}

type SystemInfo struct {
	SerialNum       string `json:"serial_num"`
	Model           string `json:"model"`
	ModelFullName   string `json:"modelFullName"`
	HardwareVersion string `json:"hardware_version"`
	SoftwareVersion string `json:"software_version"`
	SysMAC          string `json:"sys_mac"`
	WANIP           string `json:"wan_ip"`
	AuthIP          string `json:"auth_ip"`
}

type Authority struct {
	Hostname   string     `json:"hostname"`
	DeviceRole string     `json:"deviceRole"`
	Sysinfo    SystemInfo `json:"sysinfo"`
}

// Session is transient and is never included in the device configuration.
type Session struct {
	Token     string
	SID       string
	SN        string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type LoginResult struct {
	Session   Session
	Authority Authority
}
