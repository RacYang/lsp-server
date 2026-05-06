package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Prompter 抽象 lobby 的"提问—输入—下一步"交互。
//
// 生产实现包裹 stdin/stdout，行式读写；
// 测试实现可以注入预设输入并捕获输出，便于表驱动覆盖每个菜单分支。
type Prompter interface {
	// Print 把任意可读文案写到输出流，行尾自带换行。
	Print(args ...any)
	// Printf 是 fmt.Sprintf + Print 的组合，自动补换行。
	Printf(format string, args ...any)
	// PrintBlank 输出一个空行，让相邻菜单视觉上分隔。
	PrintBlank()
	// AskLine 打印 label 后阻塞读取一行输入,EOF 时返回 io.EOF。
	AskLine(label string) (string, error)
}

// IOPrompter 是基于 io.Reader / io.Writer 的 Prompter 实现。
//
// 写入端通过 bufio.Writer 包裹以减少系统调用,但每次 Print/AskLine 之后都会显式 Flush;
// 读取端直接复用底层 bufio.Reader,避免在多次 ReadString 之间持有跨界预读缓冲。
type IOPrompter struct {
	in  *bufio.Reader
	out io.Writer
}

// NewIOPrompter 把任意 io 流包装成 Prompter。生产里会传 os.Stdin / os.Stdout。
func NewIOPrompter(in io.Reader, out io.Writer) *IOPrompter {
	if in == nil {
		in = strings.NewReader("")
	}
	if out == nil {
		out = io.Discard
	}
	return &IOPrompter{in: bufio.NewReader(in), out: out}
}

func (p *IOPrompter) Print(args ...any) {
	_, _ = fmt.Fprintln(p.out, args...)
}

func (p *IOPrompter) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(p.out, format+"\n", args...)
}

func (p *IOPrompter) PrintBlank() {
	_, _ = fmt.Fprintln(p.out)
}

// AskLine 打印 label 后立刻 flush,然后阻塞读取下一行。
//
// EOF 与 ctx-cancel 不在此层处理:
//   - EOF 直接以 io.EOF 上抛,调用方解释为"用户退出";
//   - 取消语义由 SIGINT 处理器 + bufio.Reader 关闭实现。
func (p *IOPrompter) AskLine(label string) (string, error) {
	if label != "" {
		_, _ = fmt.Fprint(p.out, label)
	}
	line, err := p.in.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if err != nil {
		if errors.Is(err, io.EOF) && line != "" {
			return line, nil
		}
		return line, err
	}
	return line, nil
}
