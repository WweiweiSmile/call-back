package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 这几个用例锁的是本次修复的核心：.env 的解析不能依赖进程 CWD。
// 背景：godotenv.Load() 不带参数时只认 CWD，生产的宝塔面板拉起的进程
// CWD 不是项目目录，于是整个 .env 静默失效。

// chdirTemp 切到一个新的空临时目录，并在用例结束后恢复。
//
// 不用 t.Chdir：go.mod 声明的是 go1.22，那个 API 要 go1.24 才解锁，
// 而为了一个测试去抬整个模块的语言版本不划算
func chdirTemp(t *testing.T) string {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Logf("恢复工作目录失败: %v", err)
		}
	})
	return dir
}

func TestLocateEnvFilePrefersExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "custom.env")
	if err := os.WriteFile(target, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envFileOverride, target)

	got, tried, err := locateEnvFile()
	if err != nil {
		t.Fatalf("override 指向存在的文件时不该报错: %v", err)
	}
	if got != target {
		t.Errorf("期望 %q，实际 %q", target, got)
	}
	if len(tried) != 0 {
		t.Errorf("override 命中时不该再走候选列表，实际 tried=%v", tried)
	}
}

func TestLocateEnvFileTrimsOverride(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "custom.env")
	if err := os.WriteFile(target, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 从 systemd / 面板的输入框里粘出来的路径常带首尾空白
	t.Setenv(envFileOverride, "  "+target+"  ")

	got, _, err := locateEnvFile()
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if got != target {
		t.Errorf("首尾空白应被裁掉，期望 %q，实际 %q", target, got)
	}
}

func TestLocateEnvFileFailsWhenOverrideMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.env")
	t.Setenv(envFileOverride, missing)

	got, _, err := locateEnvFile()
	if err == nil {
		t.Fatal("ENV_FILE 显式指定却读不到时必须报错，静默退回环境变量会变成下一个难查的问题")
	}
	if got != "" {
		t.Errorf("报错时不该返回路径，实际 %q", got)
	}
	if !strings.Contains(err.Error(), envFileOverride) {
		t.Errorf("错误信息里要点名 %s，方便定位，实际: %v", envFileOverride, err)
	}
}

func TestLocateEnvFileFallsBackToCwd(t *testing.T) {
	t.Setenv(envFileOverride, "")

	// 可执行文件目录优先于 CWD。测试进程的二进制在临时构建目录里，
	// 正常情况下那里不会有 .env；真有了就跳过，避免误判
	if _, err := os.Stat(candidateEnvFiles()[0]); err == nil {
		t.Skip("可执行文件目录下存在 .env，无法隔离验证 CWD 回落")
	}

	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, _, err := locateEnvFile()
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if got != ".env" {
		t.Errorf("期望回落到 CWD 的 .env，实际 %q", got)
	}
}

func TestLocateEnvFileReportsCandidatesWhenAbsent(t *testing.T) {
	t.Setenv(envFileOverride, "")
	chdirTemp(t) // 空目录，两个候选都不存在

	got, tried, err := locateEnvFile()
	if err != nil {
		t.Fatalf("没有 .env 是合法的部署方式，不该报错: %v", err)
	}
	if got != "" {
		t.Errorf("期望空路径，实际 %q", got)
	}
	// tried 会原样进启动日志，必须覆盖全部候选，否则运维照着日志排查会漏
	if len(tried) != len(candidateEnvFiles()) {
		t.Errorf("tried 应覆盖全部候选，期望 %d 项，实际 %v", len(candidateEnvFiles()), tried)
	}
}

func TestCandidateEnvFilesEndsWithCwd(t *testing.T) {
	candidates := candidateEnvFiles()
	if len(candidates) != 2 {
		t.Fatalf("期望 2 个候选（可执行文件目录 + CWD），实际 %v", candidates)
	}
	if candidates[len(candidates)-1] != ".env" {
		t.Errorf("CWD 的 .env 必须作为兜底候选排在最后，实际 %v", candidates)
	}
	// 锚点顺序决定了优先级：二进制所在目录更接近"应用的家"，
	// 而 CWD 只是启动方式的副产物
	if !strings.HasSuffix(candidates[0], string(filepath.Separator)+".env") {
		t.Errorf("第一个候选应是可执行文件目录下的 .env，实际 %q", candidates[0])
	}
}
