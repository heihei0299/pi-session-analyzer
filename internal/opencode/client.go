package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	RPCURL = "https://opencode.ai/_server"

	FnWorkspaces   = "def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f"
	FnMonthlyCosts = "15702f3a12ff8bff357f8c2aa154a17e65b746d5f6b96adc9002c86ee0c15205"
	FnUsageHistory = "bfd684bfc2e4eed05cd0b518f5e4eafd3f3376e3938abb9e536e7c03df831e5c"
)

type Client struct {
	auth       string
	httpClient *http.Client
}

func NewClient(auth string) *Client {
	return &Client{
		auth: strings.TrimSpace(auth),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) buildCookieHeader() string {
	raw := c.auth
	var cookieBase string
	if strings.HasPrefix(raw, "auth=") || strings.Contains(raw, "auth=") {
		cookieBase = raw
	} else {
		cookieBase = "auth=" + raw
	}

	ocLocale := os.Getenv("oc_locale")
	if ocLocale == "" {
		ocLocale = os.Getenv("OC_LOCALE")
	}
	if ocLocale != "" && !strings.Contains(cookieBase, "oc_locale") {
		return cookieBase + "; oc_locale=" + ocLocale
	}
	return cookieBase
}

func (c *Client) rpc(fnID string, args []any) (any, error) {
	if c.auth == "" {
		return nil, fmt.Errorf("认证失效: 缺少 OpenCode auth，请设置 OPENCODE_AUTH 或传入 --auth（凭证过期/缺失）")
	}

	cookieHeader := c.buildCookieHeader()

	// 对 usageHistory 和 monthlyCosts 使用 GET 模式
	isUsageOrCosts := (fnID == FnUsageHistory || fnID == FnMonthlyCosts)
	if isUsageOrCosts {
		argsBytes, _ := json.Marshal(args)
		getURL := fmt.Sprintf("%s?id=%s&args=%s", RPCURL, url.QueryEscape(fnID), url.QueryEscape(string(argsBytes)))

		req, err := http.NewRequest("GET", getURL, nil)
		if err != nil {
			return nil, err
		}

		instanceVal := "server-fn:1"
		if fnID == FnMonthlyCosts {
			instanceVal = "server-fn:0"
		}
		var workspaceID string
		if len(args) > 0 {
			workspaceID = fmt.Sprintf("%v", args[0])
		}

		req.Header.Set("Cookie", cookieHeader)
		req.Header.Set("X-Server-Id", fnID)
		req.Header.Set("X-Server-Instance", instanceVal)
		req.Header.Set("Referer", fmt.Sprintf("https://opencode.ai/workspace/%s/usage", workspaceID))
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("网络超时: 请求 OpenCode 超时，请检查网络 — %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, fmt.Errorf("认证失效: OpenCode 凭证已过期或无效（HTTP %d），请刷新 auth cookie（凭证过期）", resp.StatusCode)
		}
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("请求失败: OpenCode 返回 HTTP 404（Function ID 可能已随前端发版更换）")
		}
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("服务器错误: OpenCode 服务异常（HTTP %d），请稍后重试", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return DecodeResponseText(string(bodyBytes))
	}

	// POST 模式
	payload := EncodePayload(args)
	req, err := http.NewRequest("POST", RPCURL, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("X-Server-Id", fnID)
	req.Header.Set("X-Server-Instance", "server-fn:0")
	req.Header.Set("Referer", "https://opencode.ai/")
	req.Header.Set("Origin", "https://opencode.ai")
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络超时: 请求 OpenCode 超时，请检查网络 — %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败: OpenCode 返回 HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return DecodeResponseText(string(bodyBytes))
}

func (c *Client) GetWorkspaces() ([]WorkspaceInfo, error) {
	raw, err := c.rpc(FnWorkspaces, []any{})
	if err != nil {
		return nil, err
	}

	bytes, _ := json.Marshal(raw)
	var list []WorkspaceInfo
	if err := json.Unmarshal(bytes, &list); err == nil && len(list) > 0 {
		return list, nil
	}

	// 兼容单对象
	var single WorkspaceInfo
	if err := json.Unmarshal(bytes, &single); err == nil && single.ID != "" {
		return []WorkspaceInfo{single}, nil
	}
	return nil, nil
}

func (c *Client) GetMonthlyCosts(workspaceID string) (*CostsResult, error) {
	raw, err := c.rpc(FnMonthlyCosts, []any{workspaceID})
	if err != nil {
		return nil, err
	}
	bytes, _ := json.Marshal(raw)
	var res CostsResult
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetUsageHistory(workspaceID string, page int) ([]UsageRecord, error) {
	raw, err := c.rpc(FnUsageHistory, []any{workspaceID, page})
	if err != nil {
		return nil, err
	}
	bytes, _ := json.Marshal(raw)
	var records []UsageRecord
	if err := json.Unmarshal(bytes, &records); err != nil {
		return nil, err
	}
	return records, nil
}
