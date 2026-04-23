// Package fan 提供番种枚举与分数分解；Phase 1 仅覆盖川麻 MVP 常用项。
package fan

// Kind 表示番种类型。
type Kind string

const (
	KindPingHu       Kind = "ping_hu"        // 平胡
	KindDuiDuiHu     Kind = "dui_dui_hu"     // 大对子（对对胡）
	KindQingYiSe     Kind = "qing_yi_se"     // 清一色
	KindQiDui        Kind = "qi_dui"         // 七对
	KindYiGen        Kind = "yi_gen"         // 根（四张相同未杠出）
	KindGangShangKai Kind = "gang_shang_kai" // 杠上开花（Phase 1 仅占位）
)

// Item 为单个番种项。
type Item struct {
	Kind  Kind
	Fan   int    // 番数（整数番）
	Label string // 中文说明，便于日志与测试
}

// Breakdown 为番种分解结果。
type Breakdown struct {
	Items []Item
	Total int
}

// Add 追加一项并累计总番。
func (b *Breakdown) Add(kind Kind, fan int, label string) {
	if fan <= 0 {
		return
	}
	b.Items = append(b.Items, Item{Kind: kind, Fan: fan, Label: label})
	b.Total += fan
}
