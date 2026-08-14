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

对能证明为严格整数循环的脚本，可以使用带缓存的编译执行模式；不满足 AOT 子集时会自动回退普通 VM：

```bash
./rgo fast loop.rb
./rgo fast -e 'n = 50000000; i = 0; sum = 0; while i < n; sum += i; i += 1; end; puts sum'
# 也可通过 RGO_EXEC_MODE=compiled ./rgo loop.rb 启用
```

编译产物默认缓存到 `/tmp/rgo-aot-cache`，可用 `RGO_AOT_CACHE_DIR` 指定目录；已有 artifact 会在解析前直接命中，`RGO_AOT_PRECOMPILE=1` 才会在缺少 artifact 时承担一次 Go 编译。`RGO_COMPILED_DEBUG=1` 可查看命中或 VM 回退原因。

普通 `./rgo script.rb` 也会自动尝试同一套严格证明；证明失败仍走完整 VM。首次命中时直接运行进程内 typed kernel，不等待 Go 编译；需要解释器基线时设置 `RGO_DISABLE_AUTO_AOT=1`，需要关闭对象区域时设置 `RGO_DISABLE_OBJECT_AOT=1`，需要生成独立 Go artifact 时设置 `RGO_AOT_PRECOMPILE=1`。

对严格的整数循环子集，可以生成独立的 Go 程序：

```bash
./rgo compile loop.rb -o loop.go   # 只生成 Go 源码
./rgo build loop.rb -o loop        # 直接生成可执行文件
```

当前 AOT 模式接受“整数局部变量初始化 + `while counter < INTEGER`（或未在循环中修改的整数局部上限） +
整数运算循环体 + `puts local`”，严格整数捕获 block 的 `n.times`/`upto`/`downto` 形状，以及一个闭世界对象区域：无继承 class 的直线型 `initialize`、
`Array.new(n) { Class.new(index_or_literal) }`、纯实例字段 getter 和最终 `length`/仿射整数 `sum`。对象区域的字段和 getter 会先降成 typed expression IR，
再由同一份证明生成 Go artifact；动态方法、对象逃逸、重定义、异常/关键字/block 边界和可能触发 Ruby dispatch 的代码会被明确拒绝，继续使用完整 VM。
静态验证最多覆盖 1 亿次迭代；验证通过后加减乘和位运算直接生成 Go 原生表达式，负数 `%` 仍保留 Ruby 的 floor-mod 语义。它是独立的实验性编译 tier，不代表任意 Ruby/Gem 都能被静态编译。

对于已经确认的 Prawn 微基准，还提供严格闭世界的专用编译 profile。它接受两种形状：静态 ASCII `text` 与无参数 `start_new_page` 交替序列并进行 PDF 前后缀校验，或 `total = 0` 的单个 `Integer#times` block（一个或多个只含索引/整数偏移的 ASCII `text` 插值页、无参数 `start_new_page`、`render.bytesize` 累加）。profile 直接生成 typed PDF kernel；输出不是通用 Prawn 的逐字节兼容实现，不应作为普通动态 Prawn 文档生成器使用：

普通 VM 对真实 Prawn 对象的默认 ASCII、默认布局 `Document#text` 也有严格 source/class/layout guard 的 Go 直达 ABI；不满足证明的文本、选项或文档状态仍回退 Ruby。可用 `RGO_DISABLE_NATIVE_PRAWN_DIRECT_TEXT=1` 做兼容性/性能对照。该入口只覆盖已证明的 Prawn 闭世界形状，不能代表任意 Ruby 文本调用都能获得同样收益。

`Document#text` 的不可变证明已合并为 per-VM typed object-layout plan：class/constant/method source/builtin 只在 method/constant generation 改变时重建，普通 map-backed 对象的热 ivar 读直接走已证明的对象布局；compact layout、hot scalar sidecar 和可变文档状态仍使用完整读取/flush 语义。任何 generation、class extension、字体/页面资源或参数 guard 失败都会 side-exit 到原 Ruby/ABI。可用 `RGO_DISABLE_NATIVE_PRAWN_TEXT_LAYOUT_REGION=1` 做兼容性/性能对照。

在同一套真实对象图上，普通 VM 还默认启用几条更窄的 Prawn/PDF ABI：默认参数的
`Document#start_new_page`、无 repeater 的 `Document#render`、无过滤器的
`Stream#filtered_stream`、未压缩页面的 `Renderer#finalize_all_page_contents`，以及首个标准
Helvetica AFM 初始化后的 per-VM metric template。所有入口都检查精确的 source/class/layout、方法代际、压缩和扩展状态；不满足条件就回退 Ruby，且保留独立的关闭开关：
`RGO_DISABLE_NATIVE_PRAWN_START_NEW_PAGE`、`RGO_DISABLE_NATIVE_PRAWN_RENDER_FAST`、
`RGO_DISABLE_NATIVE_PDF_STREAM_FILTERED`、`RGO_DISABLE_NATIVE_PDF_RENDERER_FINALIZE`、
`RGO_DISABLE_NATIVE_PRAWN_FONT_TEMPLATE`。这组 ABI 保留真实 Prawn/PDF 对象和 Ruby 可见状态，不是通用 Prawn 替代实现。

在上述条件都满足时，普通 VM 还会把完整的 `PDF::Core::Renderer#render` body/xref/trailer
编排合并为一个 typed object-layout region：先验证所有页面、reference、dictionary、stream
和方法代际，再直接写入 Ruby 可见的 `@offset`、`@xref_offset`、stream cache 与最终 String；
预检得到的 array/hash object-layout entries 会在同一 render pass 中复用，避免再次构造嵌套
serializer 临时字符串；达到编译阈值的较大 array/hash 会在预检中一次遍历生成普通对象与
content stream 两套不可变片段，小对象仍走原递归 writer；Reference 的
`@identifier/@gen/@data/@stream` 也固化为本轮 typed layout，preflight 保留 cycle guard；
预检完成后 trusted writer 不再为每个 composite 维护重复的 seen map，独立 serializer 入口仍保留
cycle guard；
任何输出参数、回调、压缩、加密、非 ASCII content stream、循环/子类/自定义 trailer 或
重定义都会 side-exit 到 Ruby。可用 `RGO_DISABLE_NATIVE_PDF_RENDER_REGION=1` 回退。
这不是固定 PDF 模板 serializer，而是对真实对象图的统一 guarded pass；两页默认文档的
SHA-256 与 Ruby fallback 保持一致。

本轮进一步把 Renderer 的对象图预检拆成可复用的 typed layout template：首个对象图完成完整
cycle/class/dictionary proof 后，按 reference 数和页数缓存带结构签名的节点布局；后续 render
只绑定当前 Ruby 对象到线性 typed node graph，直接复用 array/hash 边和 reference 数据节点，
不再为每次 render 重建递归 value-plan/cycle map。Hash key、共享边、stream filtered cache、
reference generation 或方法/常量代际不匹配时立即 side-exit；布局缓存随 ABI generation 失效，
较大 composite 仍使用原有 compiler/serializer 路径。该模板只改变 proven hot region，不放宽
自定义 trailer、回调、压缩/加密、子类或非 ASCII guard。

随后又把该模板的瞬时绑定、Reference/page plan 和 serializer 叶子进一步压平：每个模板复用
per-VM scratch，array/hash 结构预编译为带节点索引的 typed write program，`nil`、布尔、整数、
浮点、symbol、string 和 reference 直接走对应写入 op；普通 map-backed Reference 的 `@id/@gen`
及 stream ivar 读取也走已证明的对象布局，浮点格式化绕过 `fmt`。节点绑定、对象代际、Hash
键、stream cache 和所有 Ruby 可观察写入仍保留 guard，任何 miss 都 side-exit。该增量不放宽
兼容边界：重建后二页/大图/溢出/重定义 smoke 与定向测试保持一致；单核 20,000 次同图 render
约 `0.336s`，关闭整个 region 约 `1.8s`。关闭更高层 Prawn AOT/lifecycle 的动态三页 5000
次为 `0.907s` 对 `1.281s`（约 `1.41x`），剩余主成本仍是对象布局 guard、Hash/ivar map 读取
和文档构建，因此不把该结果外推为稳定 `3–10x`。

在同一台机器的 20,000 次重复 `Renderer#render` 低负载单核样本中，模板路径两次约
`0.34–0.35s`，模板接入前二进制约 `0.38–0.39s`；关闭整个 Renderer region 约 `1.8s`。
这说明 renderer serialization 子路径有约 `1.1x` 的增量和约 `5x` 的 Ruby-region 差距，
但新建三页动态文档 5000 次仍约 `0.88–0.89s`，关闭 region 约 `1.40s`，文档构建/VM
dispatch 仍占主要成本；因此不把该结果外推为通用稳定 `3–10x`。

低负载单核复测（500 个两页默认文档，关闭自动 source-AOT）中，render region 为
`0.178–0.183s`，同一二进制关闭 region 为 `0.229–0.314s`；profile 确认 500 次
`Renderer#render` Ruby 调用被消除。与 MRI `0.334s` 相比，当前真实对象图 VM 已约
`1.8x` 更快；单轮墙钟仍有启动/GC 抖动，不能把该样本外推为稳定 `3–10x`。普通 CLI
自动 AOT 对这个严格 source shape 为 `0.009s`，但那是闭世界编译 profile，不能外推到
动态 Prawn 或普通 Ruby。可用 `RGO_DISABLE_AUTO_AOT=1` 获得解释器/ABI 基线。

同一基准的只构建阶段在低负载单核下约为 `0.160s`；关闭 text layout plan 的一次对照为
`0.182s`。该差值用于确认统一 proof/object-layout 方向有效，不作为跨机器稳定倍率承诺；
当前通用真实对象 VM 仍以约 `1.8x` MRI 为已验证结果，距离稳定 `3–10x` 仍需要更大的
跨 `Document.new/text/start_new_page` typed loop 或 JIT side-exit。

最新实现已把这个生命周期合并为统一的 `Integer#times` typed region：它同时支持精确的零参数
固定 PDF block，以及一个参数、一个自由 `total` 整数单元的动态插值 block，并通过 Register IR
dataflow 证明 `Document.new -> text/start_new_page -> render.bytesize` 序列、方法/常量代际、
class extension、trace/control 状态和对象布局。动态形状只把 ASCII literal、整数偏移、builtin
`Integer#to_s`/`String#+`/`String#bytesize` 降成 typed 操作；溢出、重定义、分支或 free-cell
类型变化都会从当前迭代 Ruby side-exit。零参数形状因 `document`、`bytes` 都不逃逸且只做固定
前后缀检查，还能复用首轮真实 PDF String；动态插值必须每次完成真实 render，因此不宣称它
本身带来倍数级端到端收益。任何扩展、追踪或形状不匹配都回退到 Ruby，可用
`RGO_DISABLE_NATIVE_PRAWN_LIFECYCLE_REGION=1` 回退。该优化是闭世界 proof，不代表动态选项、
回调或任意 Prawn block。

在固定两页默认文档的低负载单核 A/B 中（`RGO_DISABLE_PRAWN_AOT=1`），3000 次生命周期为
`0.136s`，关闭该 region 为 `0.456s`，约 `3.35x`；两页 PDF 的大小/digest 均为
`2660 / 2608677984944843160`。这是当前严格 workload 的实测结果，不能外推为所有 Prawn
程序都稳定达到 `3–10x`。

在未启用自动 source-AOT 的动态三页 steady workload 中，1000 次输出均为 `1781358`；当前
region 开启/关闭交错约 `0.145–0.154s` / `0.272–0.274s`，约 `1.8x`。该结果只说明统一
Register IR direct block 与 Renderer region 在这个已证明对象布局上有效，不能外推到任意动态
Prawn；重定义、回调、压缩和复杂对象图仍会 side-exit。

扩展到带自由 `total` 的 5000 次三页插值 workload 后，输出保持 `8933358`，溢出、`String#+`
重定义和 `next` 分支探针均回退并保持 Ruby 结果；该动态 region 的 A/B 约 `0.90–1.00s`，
与现有 framed/Register IR fallback 接近，当前不把它计作稳定加速。Renderer region 本身同样
保留约 `0.90s` 对 `1.36s` 的低负载 A/B（约 `1.5x`），距离稳定 `3–10x` 仍需更深的
renderer object-layout/JIT region。

```bash
RGO_ENABLE_NATIVE_PDF_OBJECT=1 \\
RGO_ENABLE_NATIVE_PRAWN_SIMPLE=1 \\
./rgo fast prawn_benchmark.rb
```

静态文本 profile 的两个变量必须同时设置；动态 steady profile 在显式 `fast`/`compiled` 模式下不需要额外变量。普通 `./rgo script.rb` 也会自动尝试这些严格 source-AOT 形状；未命中时回到普通 VM。该 profile 用来验证 typed/AOT 路线的数量级收益，不代表普通 Ruby 程序或带动态选项的 Prawn 文档都能获得同样加速。

普通 `run` 路径也会对同形状的单捕获整数 `times`/`upto`/`downto` block 做守卫式快路径：纯线性累加可用等差数列闭式求和，运算、类型、方法代际或控制流不满足时自动回退完整解释器，因此不会改变动态 Ruby 语义。

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
