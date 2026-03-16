# 阶段 6-10: 剩余阶段概要设计

本文档包含阶段 6-10 的概要设计。这些阶段将在前 5 个阶段完成后根据实际情况进行详细设计。

---

## 阶段 6: Symbol & Range 运行时

**优先级**: P1  
**预计时间**: 1 周  
**依赖**: 阶段 1 (Enumerable)

### 目标
实现完整的 Symbol 和 Range 对象，使其可以在表达式中正常使用。

### 关键任务
1. **Symbol 运行时** (Day 1-3)
   - Symbol 对象结构和内部表
   - Symbol 方法: to_s, to_sym, length, upcase, downcase
   - Symbol 字面量支持: `:symbol`
   - Symbol 在 Hash 中作为键

2. **Range 运行时** (Day 4-5)
   - Range 对象结构 (begin, end, exclude_end?)
   - Range 方法: each, to_a, cover?, include?
   - Range 字面量: `1..10`, `1...10`
   - Range 在迭代中的使用

### 验收标准
- [ ] 通过 symbol/ 下 80%+ spec
- [ ] 通过 range/ 下 80%+ spec
- [ ] Symbol 可作为 Hash 键
- [ ] Range 可用于迭代

---

## 阶段 7: Regexp 引擎

**优先级**: P1  
**预计时间**: 3 周  
**依赖**: 阶段 3 (String methods)

### 目标
实现基础正则表达式支持，集成 Go 的 regexp 包。

### 关键任务
1. **正则表达式字面量** (Week 1)
   - Lexer 支持: `/pattern/flags`
   - Parser 支持: RegexpLiteral AST
   - Compiler 支持: OpRegexp
   - 标志: i (ignore case), m (multiline), x (extended)

2. **Regexp 对象和方法** (Week 2)
   - Regexp 类实现
   - 方法: =~, match, match?, names, source
   - MatchData 对象
   - MatchData 方法: [], captures, named_captures

3. **String 正则方法** (Week 3)
   - String#scan 正则版本
   - String#gsub 正则版本
   - String#sub 正则版本
   - String#split 正则版本
   - String#match, String#match?

### 技术要点
```go
// 集成 Go regexp
import "regexp"

type RegexpData struct {
    Pattern string
    Flags   string
    Compiled *regexp.Regexp
}

// 编译正则表达式
func compileRegexp(pattern, flags string) (*regexp.Regexp, error) {
    // 转换 Ruby 标志到 Go 标志
    goPattern := pattern
    if strings.Contains(flags, "i") {
        goPattern = "(?i)" + goPattern
    }
    if strings.Contains(flags, "m") {
        goPattern = "(?m)" + goPattern
    }
    
    return regexp.Compile(goPattern)
}
```

### 验收标准
- [ ] 通过 regexp/ 下 60%+ spec
- [ ] 基础模式匹配工作
- [ ] String 正则方法可用
- [ ] MatchData 对象正确

---

## 阶段 8: Module 系统

**优先级**: P1  
**预计时间**: 2 周  
**依赖**: 无

### 目标
完善模块系统，实现 include/extend/prepend 和方法查找链。

### 关键任务
1. **Module 定义完善** (Week 1, Day 1-3)
   - module 关键字解析
   - Module 对象结构
   - 模块方法定义
   - module_function

2. **Include/Extend/Prepend** (Week 1, Day 4-5)
   - include: 混入实例方法
   - extend: 混入类方法
   - prepend: 前置混入
   - 方法查找顺序 (MRO)

3. **高级特性** (Week 2)
   - included/extended/prepended 钩子
   - Module#ancestors
   - Module#included_modules
   - 模块嵌套和命名空间

### 方法查找顺序 (MRO)
```
Class
  ↓
Prepended Modules (逆序)
  ↓
Class Methods
  ↓
Included Modules (逆序)
  ↓
Superclass
```

### 验收标准
- [ ] 通过 module/ 下 80%+ spec
- [ ] include/extend/prepend 工作
- [ ] 方法查找顺序正确
- [ ] 模块钩子触发

---

## 阶段 9: IO & File

**优先级**: P2  
**预计时间**: 3 周  
**依赖**: 阶段 5 (Exception)

### 目标
实现基础文件操作，支持 require/require_relative。

### 关键任务
1. **File 类** (Week 1)
   - File.open, File.read, File.write
   - File#close, File#read, File#write
   - File#each_line, File#readlines
   - File 测试方法: exist?, directory?, file?

2. **IO 类** (Week 2)
   - IO 基类实现
   - IO#puts, IO#print, IO#gets
   - IO#read, IO#write
   - STDIN, STDOUT, STDERR 常量

3. **Dir 类和 require** (Week 3)
   - Dir.entries, Dir.glob
   - Dir.pwd, Dir.chdir
   - require 实现
   - require_relative 实现
   - $LOAD_PATH 支持

### 技术要点
```go
// File 操作
import "os"

fileClass.DefineMethod("open", &object.Method{
    Name:  "open",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        if len(args) < 1 {
            // 抛出 ArgumentError
            return R.NilVal
        }
        
        filename, _ := args[0].Data.(string)
        mode := "r"
        if len(args) > 1 {
            mode, _ = args[1].Data.(string)
        }
        
        file, err := os.OpenFile(filename, parseMode(mode), 0644)
        if err != nil {
            // 抛出 IOError
            return R.NilVal
        }
        
        fileObj := &object.EmeraldValue{
            Type:  object.ValueFile,
            Data:  file,
            Class: R.Classes["File"],
        }
        
        if block != nil {
            defer file.Close()
            return CallBlock(block, fileObj)
        }
        
        return fileObj
    },
})
```

### 验收标准
- [ ] 通过 file/ 下 60%+ spec
- [ ] 通过 io/ 下 60%+ spec
- [ ] 文件读写工作
- [ ] require 可加载文件

---

## 阶段 10: Rails 兼容性

**优先级**: P3  
**预计时间**: 长期迭代  
**依赖**: 阶段 1-9 全部

### 目标
能够运行简单的 Rails 应用，加载 ActiveSupport 核心扩展。

### 关键任务
1. **标准库实现** (Month 1-2)
   - json 库 (JSON.parse, JSON.generate)
   - erb 库 (ERB 模板引擎)
   - net/http 库 (HTTP 客户端)
   - uri 库 (URI 解析)
   - time 库 (Time 扩展)
   - date 库 (Date/DateTime)

2. **Gem 加载机制** (Month 3)
   - Gem.path 支持
   - Bundler 基础支持
   - gemspec 解析
   - 依赖解析

3. **ActiveSupport 兼容** (Month 4-5)
   - Core Extensions (String, Array, Hash)
   - ActiveSupport::Concern
   - ActiveSupport::Callbacks
   - ActiveSupport::Configurable
   - Time zones

4. **Rack 兼容层** (Month 6)
   - Rack 接口实现
   - Rack::Request, Rack::Response
   - 中间件支持
   - 简单的 Rack 应用运行

5. **Rails 核心** (Month 7+)
   - ActionDispatch 路由
   - ActionController 基础
   - ActionView 渲染
   - ActiveRecord 查询接口 (可选)

### 里程碑
1. **M1**: 运行 "Hello World" Rack 应用
2. **M2**: 加载 ActiveSupport
3. **M3**: 运行简单的 Rails 控制器
4. **M4**: 渲染 ERB 模板
5. **M5**: 处理路由和参数

### 验收标准
- [ ] 运行 Rack "Hello World"
- [ ] 加载 ActiveSupport 无错误
- [ ] 运行简单的 Rails 应用
- [ ] 渲染 ERB 模板
- [ ] 处理 HTTP 请求

---

## 总体时间线

```
Week 1-2:   阶段 1 - Enumerable 模块
Week 3-4:   阶段 2 - Array 方法
Week 5-6:   阶段 3 - String 方法
Week 7:     阶段 4 - Hash 方法
Week 8-9:   阶段 5 - Exception 系统
Week 10:    阶段 6 - Symbol & Range
Week 11-13: 阶段 7 - Regexp 引擎
Week 14-15: 阶段 8 - Module 系统
Week 16-18: 阶段 9 - IO & File
Week 19+:   阶段 10 - Rails 兼容 (长期)
```

## 下一步行动

1. ✅ 完成所有设计文档
2. ⏭️ 审核设计文档
3. ⏭️ 开始实施阶段 1: Enumerable 模块
4. ⏭️ 建立 CI/CD 流程
5. ⏭️ 设置 spec 自动化测试

---

**文档版本**: 1.0  
**创建时间**: 2026-03-16  
**状态**: 待审核
