package domain

import "math"

// Usage 表示一条计入口径的 assistant 消息消耗 (message.usage)
type Usage struct {
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	CacheRead   float64 `json:"cacheRead"`
	CacheWrite  float64 `json:"cacheWrite"`
	Reasoning   float64 `json:"reasoning"`
	TotalTokens float64 `json:"totalTokens"`
	Cost        *Cost   `json:"cost,omitempty"`
}

type Cost struct {
	Total float64 `json:"total"`
}

// Totals 表示聚合汇总指标
type Totals struct {
	Requests    int     `json:"requests"`
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	CacheRead   float64 `json:"cacheRead"`
	CacheWrite  float64 `json:"cacheWrite"`
	Reasoning   float64 `json:"reasoning"`
	TotalTokens float64 `json:"totalTokens"`
	Cost        float64 `json:"cost"`
	CacheRate   float64 `json:"cacheRate"`
	CostStatus  string  `json:"costStatus,omitempty"`
}

type GroupBy string

const (
	GroupByModel    GroupBy = "model"
	GroupByCwd      GroupBy = "cwd"
	GroupByModelCwd GroupBy = "model,cwd"
)

type Period string

const (
	PeriodDay   Period = "day"
	PeriodWeek  Period = "week"
	PeriodMonth Period = "month"
)

type SessionRow struct {
	Totals
	SessionId       string `json:"sessionId"`
	Timestamp       string `json:"timestamp"`
	Cwd             string `json:"cwd"`
	Model           string `json:"model"`
	FileName        string `json:"fileName,omitempty"`
	DisplayName     string `json:"displayName,omitempty"`
	CwdNorm         string `json:"cwdNorm,omitempty"`
	IsTask          bool   `json:"isTask,omitempty"`
	ParentSessionId string `json:"parentSessionId,omitempty"`
	Source          string `json:"source,omitempty"`
}

type RequestRow struct {
	Totals
	SessionId       string `json:"sessionId"`
	Timestamp       string `json:"timestamp"`
	Model           string `json:"model"`
	DisplayName     string `json:"displayName,omitempty"`
	Source          string `json:"source,omitempty"`
	SourceSessionId string `json:"sourceSessionId,omitempty"`
}

type GroupRow struct {
	Totals
	Model string `json:"model,omitempty"`
	Cwd   string `json:"cwd,omitempty"`
}

type PeriodRow struct {
	Totals
	Period string `json:"period"`
}

func EmptyTotals() Totals {
	return Totals{}
}

func AddUsage(t *Totals, u Usage) {
	t.Requests++
	t.Input += toFinite(u.Input)
	t.Output += toFinite(u.Output)
	t.CacheRead += toFinite(u.CacheRead)
	t.CacheWrite += toFinite(u.CacheWrite)
	t.Reasoning += toFinite(u.Reasoning)
	if u.Cost != nil {
		t.Cost += toFinite(u.Cost.Total)
	}
}

// FinalizeTotals 收尾计算：根据 ADR-0002，总 token = input + cacheRead + output；缓存率 = cacheRead / (input + cacheRead)
func FinalizeTotals(t *Totals) {
	totalInput := t.Input + t.CacheRead
	t.TotalTokens = totalInput + t.Output
	if totalInput == 0 {
		t.CacheRate = 0
	} else {
		t.CacheRate = t.CacheRead / totalInput
	}
}

func toFinite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
