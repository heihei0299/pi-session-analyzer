package opencode

const AuditComparisonExplanation = "本地统计仅含 pi 会话中计入的 assistant 消息（MessageTimeRange），未计入内部 compaction/分支同步等非会话请求及非 pi 客户端请求；OpenCode 官方账单含全部扣费请求，差额为预期结构性差异。localCost 为本地 usage.cost.total 求和（若未定价则可能为 0），opencode 成本为官方 cost 求和。"

func ComputeOpencodeTotals(records []UsageRecord) OpencodeTotals {
	var input, output, cacheRead, cacheWrite, reasoning, cost float64
	for _, r := range records {
		input += r.InputTokens
		output += r.OutputTokens
		cacheRead += r.CacheReadTokens
		cacheWrite += r.CacheWrite5mTokens + r.CacheWrite1hTokens
		reasoning += r.ReasoningTokens

		rawCost := r.Cost
		if rawCost > 100 {
			cost += rawCost / 1e8
		} else {
			cost += rawCost
		}
	}
	totalTokens := input + cacheRead + output
	return OpencodeTotals{
		Requests:    len(records),
		Input:       input,
		Output:      output,
		CacheRead:   cacheRead,
		CacheWrite:  cacheWrite,
		Reasoning:   reasoning,
		TotalTokens: totalTokens,
		Cost:        cost,
	}
}

func BuildAudit(localTotals LocalTotals, opencodeRecords []UsageRecord, year, month int) AuditResult {
	opTotals := ComputeOpencodeTotals(opencodeRecords)

	diff := AuditDiff{
		Requests: opTotals.Requests - localTotals.Requests,
		Tokens:   opTotals.TotalTokens - localTotals.TotalTokens,
		Cost:     opTotals.Cost - localTotals.Cost,
	}

	var reqRate, tokRate, costRate float64
	if opTotals.Requests > 0 {
		reqRate = float64(diff.Requests) / float64(opTotals.Requests)
	}
	if opTotals.TotalTokens > 0 {
		tokRate = diff.Tokens / opTotals.TotalTokens
	}
	if opTotals.Cost > 0 {
		costRate = diff.Cost / opTotals.Cost
	}

	return AuditResult{
		Year:           year,
		Month:          month,
		LocalTotals:    localTotals,
		OpencodeTotals: opTotals,
		Diff:           diff,
		DiffRate: AuditDiffRate{
			Requests: reqRate,
			Tokens:   tokRate,
			Cost:     costRate,
		},
		Comparison: AuditComparisonExplanation,
	}
}
