package acConfig

type NetworkConnectGlobalResp struct {
	Enable string `json:"enable"`
	Param  []struct {
		Datatype string `json:"datatype"`
		Value    string `json:"value"`
	} `json:"param"`
	NetworkID   string `json:"networkId"`
	GroupID     string `json:"groupId"`
	SubConfigID string `json:"subConfigId"`
	Version     string `json:"version"`
	ConfigID    string `json:"configId"`
	ConfigTime  string `json:"configTime"`
	CurrentTime string `json:"currentTime"`
}
