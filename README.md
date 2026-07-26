# RGo

[![Go](https://github.com/GoLangDream/rgo/actions/workflows/test.yml/badge.svg)](https://github.com/GoLangDream/rgo/actions/workflows/test.yml)

RGo 是一个用 Go 实现的实验性 Ruby 运行时，当前以 Ruby 4.0.0 兼容性为目标。项目包含 Lexer、Parser、字节码编译器、虚拟机、核心类库、命令行工具，以及内置的 MSpec/RubySpec 测试支持。

> RGo 仍处于开发阶段，适合语言实现研究、兼容性实验和项目贡献。目前不建议用于生产环境，也不能替代完整的 CRuby 标准库与原生扩展生态。

## 当前状态

截至 2026-07-25：

- Go 全仓测试通过：`./scripts/safe_go_test.sh ./...`
- VM 完整测试通过，此前 13 个独立回归已清零
- RubySpec Language：80/80 文件通过
- RubySpec 全树：3,256 个文件通过、526 个文件被平台或版本 guard 跳过、25 个文件存在行为失败、2 个文件超时
- RubySpec 已执行 31,639 个 examples，其中 53 个 failures
- Rails 兼容性尚未建立可复现门禁；仓库当前不包含 Rails 源码树

完整 RubySpec 报告见 [`reports/spec-status/ruby-spec-full.csv`](reports/spec-status/ruby-spec-full.csv)，具体开发缺口见 [`TODO.md`](TODO.md)。

## 构建

需要 Go 1.24.3 或兼容版本。Linux 上启用 CGO 时，RGo 会在运行期尝试加载 `libonig` 以提高 Ruby 正则表达式兼容性；不可用时会使用内置回退路径。

```bash
make build
./rgo --version
```

预期版本输出：

```text
ruby 4.0.0 (rgo) [linux-amd64]
```

## 使用

运行一段 Ruby 代码：

```bash
./rgo -e 'puts "hello from rgo"'
```

运行 Ruby 文件：

```bash
./rgo run example.rb
# 也支持：
./rgo example.rb
```

运行一个 RubySpec/MSpec 文件：

```bash
./rgo test vendor/ruby/spec/language/return_spec.rb
```

查看命令帮助：

```bash
./rgo help
```

## 测试

项目默认使用低并发测试脚本，避免 Go 编译和大量 spec 进程造成资源峰值：

```bash
# 全部 Go 测试
make test

# 格式、vet 和 Go 测试
make check

# 完整 RubySpec，串行执行并生成 CSV 报告
make full-ruby-spec
```

也可以只检查单个 RubySpec 目录：

```bash
RGO_SPEC_TIMEOUT=5 \
  ./scripts/spec_status.sh \
  vendor/ruby/spec/language \
  /tmp/rgo-language.csv
```

## 项目结构

```text
cmd/rgo/       命令行入口与 spec runner
pkg/lexer/     Ruby 词法分析
pkg/parser/    AST 与语法分析
pkg/compiler/  字节码编译
pkg/vm/        虚拟机与控制流
pkg/core/      Ruby 核心类和标准库兼容层
scripts/       低资源测试与兼容性门禁
vendor/ruby/   上游 RubySpec/MSpec
```

## 已知边界

- 标准库覆盖仍不完整，部分实现是面向当前 RubySpec 可观察行为的兼容层。
- C 扩展、完整进程/线程语义、Marshal、Module、Delegate、部分 Thread 行为仍有缺口。
- RubySpec 的 zero-example 文件表示被版本、平台或能力 guard 跳过，不能视为已实现。
- `ruby_exe` 部分场景使用轻量模拟；需要真实子进程语义的兼容性仍需继续收敛。

## 贡献

提交修改前请先阅读 [`AGENTS.md`](AGENTS.md)，并至少运行与改动相关的聚焦测试及 `make test`。兼容性修复应尽量附带最小 Go 回归测试或对应 RubySpec 证据。

## 许可证

Apache License 2.0，详见 [`LICENSE`](LICENSE)。
