package devSta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

// GetUserList retrieves all online wired and wireless terminals using timely data.
func GetUserList(ctx context.Context, c *client.Client, sid string) (*UserList, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, errors.New("user_list requires a sid")
	}
	request := client.ApiCommonReq{
		Method: "devSta.get",
		Params: DevStaReq{
			Module: "user_list", NoParse: true, Device: "pc",
			Data: struct {
				DevType  string `json:"devType"`
				DataType string `json:"dataType"`
			}{DevType: "all", DataType: "timely"},
		},
	}
	var raw json.RawMessage
	if err := c.Call(ctx, "cmd", request, sid, &raw); err != nil {
		return nil, fmt.Errorf("get user_list: %w", err)
	}
	return decodeUserList(raw)
}

func decodeUserList(raw json.RawMessage) (*UserList, error) {
	raw = bytes.TrimSpace(raw)
	// noParse preserves the module's JSON text inside the RPC data string.
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, errors.New("invalid user_list JSON text")
		}
		raw = []byte(text)
	}
	var data struct {
		Code  json.RawMessage `json:"code"`
		Total json.RawMessage `json:"total"`
		List  json.RawMessage `json:"list"`
		Msg   string          `json:"msg"`
		Err   string          `json:"err"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, errors.New("invalid user_list response")
	}
	if len(data.Code) > 0 {
		code, err := decodeInteger(data.Code)
		if err != nil {
			return nil, errors.New("invalid user_list code")
		}
		if code != 0 {
			message := data.Msg
			if message == "" {
				message = data.Err
			}
			return nil, fmt.Errorf("get user_list: %w", &client.APIError{Code: code, Message: message})
		}
	}
	list := bytes.TrimSpace(data.List)
	if len(list) == 0 || list[0] != '[' {
		return nil, errors.New("user_list response is missing a terminal list")
	}
	var result UserList
	if err := json.Unmarshal(list, &result.List); err != nil {
		return nil, errors.New("invalid user_list terminals")
	}
	result.Total = len(result.List)
	if len(data.Total) > 0 {
		total, err := decodeInteger(data.Total)
		if err != nil || total < 0 {
			return nil, errors.New("invalid user_list total")
		}
		result.Total = total
	}
	return &result, nil
}

func decodeInteger(raw json.RawMessage) (int, error) {
	var value Value
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	return strconv.Atoi(string(value))
}
