package render

import "github.com/gdamore/tcell/v2"

// Semantic 语义色彩，跨场景统一。
type Semantic int

const (
	SemDefault  Semantic = iota // 终端默认前景/背景
	SemEmphasis                 // 反转视频，选中/焦点/当前玩家
	SemDim                      // 灰色，次要信息、历史记录
	SemDanger                   // 红色，离线、断连、错误
	SemSuccess                  // 绿色，在线、已准备、成功
	SemWarning                  // 黄色，倒计时 < 5 秒、过期
)

// Style 把语义映射为 tcell.Style。
func Style(s Semantic) tcell.Style {
	switch s {
	case SemEmphasis:
		return tcell.StyleDefault.Reverse(true)
	case SemDim:
		return tcell.StyleDefault.Foreground(tcell.ColorGray)
	case SemDanger:
		return tcell.StyleDefault.Foreground(tcell.ColorRed)
	case SemSuccess:
		return tcell.StyleDefault.Foreground(tcell.ColorGreen)
	case SemWarning:
		return tcell.StyleDefault.Foreground(tcell.ColorYellow)
	default:
		return tcell.StyleDefault
	}
}

// DefaultStyle 返回终端默认样式。
func DefaultStyle() tcell.Style { return tcell.StyleDefault }
