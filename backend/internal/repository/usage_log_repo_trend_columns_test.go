package repository

import (
	"os"
	"strings"
	"testing"
)

// 所有趋势查询都由同一个 scanTrendRows 扫描。给扫描器加一列却漏改其中一条查询，
// 编译和单元测试都不会报错——只有连上真库跑到那条分支才会炸。这里用源码级检查
// 把「每条查询的列数 == 扫描器的目标数」钉死。
func TestTrendQueriesMatchScanTrendRows(t *testing.T) {
	source, err := os.ReadFile("usage_log_repo_trend.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	text := string(source)

	want := countScanDestinations(t, text)
	if want == 0 {
		t.Fatal("在 scanTrendRows 里没找到任何 &row.X 扫描目标，检查这个测试是否还跟得上实现")
	}

	selects := extractTrendSelectLists(text)
	if len(selects) < 4 {
		t.Fatalf("只解析出 %d 条趋势查询，预期至少 4 条（原始日志 2 条 + 预聚合表 2 条）", len(selects))
	}
	for _, columns := range selects {
		if len(columns) != want {
			t.Errorf("趋势查询有 %d 列，scanTrendRows 扫 %d 个字段；列表：%v", len(columns), want, columns)
		}
	}
}

// 预聚合表里的列叫 total_requests，requests 只是同一个 SELECT 列表中的输出别名。
// PostgreSQL 不允许在别的表达式里引用这种别名，写成 requests 会在运行时报
// column "requests" does not exist。
func TestAggregateTrendQueriesDoNotReuseSelectAliases(t *testing.T) {
	source, err := os.ReadFile("usage_log_repo_trend.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	for _, columns := range extractTrendSelectLists(string(source)) {
		for _, column := range columns {
			expression, alias, hasAlias := strings.Cut(column, " as ")
			if !hasAlias || !strings.Contains(expression, "CASE") {
				continue
			}
			_ = alias
			if strings.Contains(expression, "total_duration_ms") && !strings.Contains(expression, "total_requests") {
				t.Errorf("平均耗时表达式引用了 SELECT 别名而不是真实列：%s", column)
			}
		}
	}
}

func countScanDestinations(t *testing.T, text string) int {
	t.Helper()
	start := strings.Index(text, "func scanTrendRows(")
	if start < 0 {
		t.Fatal("找不到 scanTrendRows")
	}
	body := text[start:]
	if end := strings.Index(body, "\n}"); end > 0 {
		body = body[:end]
	}
	return strings.Count(body, "&row.")
}

// extractTrendSelectLists 取出每条趋势查询 SELECT 与 FROM 之间的列，按顶层逗号切分
// （TO_CHAR(x, 'fmt') 这类括号内的逗号不算），并剥掉 -- 注释。
func extractTrendSelectLists(text string) [][]string {
	var lists [][]string
	rest := text
	for {
		index := strings.Index(rest, "SELECT")
		if index < 0 {
			break
		}
		rest = rest[index+len("SELECT"):]
		end := strings.Index(rest, "FROM")
		if end < 0 {
			break
		}
		list := rest[:end]
		// 这个文件里还有按 key / 按 user 的时序查询，同样以 TO_CHAR(...) as date 开头，
		// 但由别的扫描器消费。喂给 scanTrendRows 的那几条以 as total_tokens 区分——
		// 其余用的是 as tokens。
		if !strings.Contains(list, "as date") || !strings.Contains(list, "as total_tokens") {
			continue
		}
		lists = append(lists, splitTopLevelColumns(list))
	}
	return lists
}

func splitTopLevelColumns(list string) []string {
	var columns []string
	var current strings.Builder
	depth := 0
	for _, line := range strings.Split(list, "\n") {
		if comment := strings.Index(line, "--"); comment >= 0 {
			line = line[:comment]
		}
		for _, char := range line {
			switch char {
			case '(':
				depth++
			case ')':
				depth--
			case ',':
				if depth == 0 {
					columns = append(columns, strings.TrimSpace(current.String()))
					current.Reset()
					continue
				}
			}
			current.WriteRune(char)
		}
		current.WriteRune(' ')
	}
	if trailing := strings.TrimSpace(current.String()); trailing != "" {
		columns = append(columns, trailing)
	}
	return columns
}
