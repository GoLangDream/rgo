# Draft: Ruby Spec Implementation Plan for RGo

## Requirements (confirmed)

**User Request**: 分析 rgo 对 ruby spec 的实现完成情况，并生成一个为了完成所有 spec,确保能正确执行的计划
- Analyze RGo's Ruby spec implementation status
- Create a comprehensive plan to complete all specs and ensure correct execution

## Initial Context (from README and TODO)

### Project Overview
- **RGo**: Go library providing Ruby-style programming experience
- **Architecture**: Lexer → Parser → Compiler → Bytecode VM
- **Test Status**: 247 tests passing (as of TODO.md)
- **Core Classes**: 17/59 implemented (incomplete)
- **Standard Library**: Nearly empty

### Current Implementation Status (from TODO.md)

**Completed (阶段 0-3)**:
- ✅ Lexer: 140+ tokens
- ✅ Parser: 40+ AST nodes
- ✅ Compiler: 80+ opcodes
- ✅ Basic control flow: if/elsif/else, while, until
- ✅ Method definitions: def with parameters
- ✅ Object system basics: class, instance variables (@foo), class variables (@@foo), global variables ($foo)
- ✅ Lambda: stabby lambda syntax (-> {})

**Partially Implemented**:
- ⚠️ case/when: Parser supports, Compiler incomplete
- ⚠️ Block/Proc: Partial support, missing yield and block_given?
- ⚠️ break/next: AST exists, partially implemented

**Not Implemented (Critical Gaps)**:
- ❌ Exception handling: begin/rescue/ensure/raise
- ❌ for loops: AST exists, Compiler missing
- ❌ unless: Token exists, not implemented
- ❌ Module system: module, include, extend, prepend
- ❌ String interpolation: "hello #{name}" runtime support
- ❌ Range: 1..10 runtime implementation
- ❌ Symbol: :symbol runtime implementation
- ❌ Regexp: /pattern/ matching engine

### Ruby Spec Structure (vendor/ruby/spec/)
- **core/**: Core class specs (Array, String, Hash, Integer, etc.)
- **language/**: Language feature specs (syntax, control flow, etc.)
- **library/**: Standard library specs (JSON, Net::HTTP, etc.)

### Priority Assessment (from TODO.md)

**P0 - Core Foundation (3-6 months)**:
1. Enumerable module - foundation for all collections
2. Array core methods - 80+ methods needed (19 implemented)
3. String core methods - 100+ methods needed (25 implemented)
4. Hash core methods - 50+ methods needed (17 implemented)
5. Kernel methods - 50+ global methods needed (~10 implemented)
6. Exception system - begin/rescue/ensure/raise + exception hierarchy

**P1 - Common Features (6-12 months)**:
7. Range object - 1..10 implementation
8. Symbol object - :symbol implementation
9. Regexp object - pattern matching engine
10. Block/Proc/Lambda - complete closure support with yield
11. Module system - include/extend/prepend
12. IO/File - basic file operations
13. String interpolation - "hello #{name}" runtime

**P2 - Advanced Features (12-18 months)**:
14. Multiple assignment and splat - a, b = 1, 2 and *args
15. Keyword arguments - def foo(a:, b: 1)
16. Method visibility - public/private/protected enforcement
17. Time/Date classes
18. Struct
19. Method/Binding objects

**P3 - Standard Library (18+ months)**:
20. JSON, Net::HTTP, FileUtils, ERB, Digest

## Research Findings

### Agent Tasks Launched
1. **bg_aae37f41**: VM and compiler capabilities analysis ✅ COMPLETED
2. **bg_e6ba8843**: Ruby spec test structure and priorities ✅ COMPLETED
3. **bg_a7b8bc40**: RGo core implementation status ✅ COMPLETED

### Ruby Spec Structure Analysis (bg_e6ba8843)

**Top 10 Most-Tested Core Classes** (by spec file count):

| Rank | Class | Spec Files | Priority |
|------|-------|------------|----------|
| 1 | **Kernel** | 117 | P0 - Foundation |
| 2 | **String** | 114 | P0 - Core |
| 3 | **Array** | 102 | P0 - Core |
| 4 | **Module** | 83 | P1 - System |
| 5 | **IO** | 80 | P1 - System |
| 6 | **Hash** | 69 | P0 - Core |
| 7 | **File** | 68 | P2 - System |
| 8 | **Integer** | 67 | P0 - Core |
| 9 | **Time** | 66 | P2 - Utility |
| 10 | **Enumerable** | 61 | P0 - Foundation |

**Additional Notable Classes**:
- Set: 52, Float: 50, Numeric: 46, Thread: 45, Env: 45
- Complex: 43, Process: 40, Exception: 39

**Language Feature Specs** (69 files):
- Control Flow: if, unless, case, while, until, for, loop, break, next, return
- Methods: def, send, method objects
- Blocks/Procs: block, proc, lambda, yield
- Classes/Modules: class, module, super, singleton_class
- Variables: local, instance, class, global, constants
- Exceptions: rescue, ensure, raise, throw
- Advanced: pattern_matching, keyword_arguments, safe_navigator

**Key Insights**:
1. **Top 5 classes** (Kernel, String, Array, Module, IO) = 496 spec files (~40% of core)
2. **Shared behaviors**: Many specs use shared examples - implementing one method satisfies multiple specs
3. **Language specs are critical**: 69 language specs cover fundamental Ruby behavior
4. **Test framework**: Ruby specs use RSpec-like syntax with MSpec framework

### RGo Core Implementation Status (bg_a7b8bc40)

**IMPLEMENTED CLASSES** (16/59 = 27%):

| Class | Methods | Spec Files | Coverage | Status |
|-------|---------|------------|----------|--------|
| **String** | 60 | 114 | ~53% | Best coverage |
| **Array** | 55 | 102 | ~54% | Best coverage |
| **Hash** | 38 | 69 | ~55% | Best coverage |
| **Object** | 29 | 119 | ~24% | Partial |
| **Integer** | 24 | 67 | ~36% | Partial |
| **Float** | 11 | 50 | ~22% | Partial |
| **Symbol** | 4 | 31 | ~13% | Minimal |
| **Range** | 2 | 35 | ~6% | Minimal |
| **Regexp** | 1 | 25 | ~4% | Minimal |
| **Module** | 0 | 83 | 0% | Skeleton only |
| **Class** | 0 | 9 | 0% | Skeleton only |
| **BasicObject** | 0 | 15 | 0% | Skeleton only |
| **TrueClass** | 0 | 9 | 0% | Skeleton only |
| **FalseClass** | 0 | 9 | 0% | Skeleton only |
| **NilClass** | 0 | 18 | 0% | Skeleton only |

**MISSING CRITICAL CLASSES** (43/59):

**P0 - Foundation** (blocks everything):
- **Enumerable** (61 spec files) — CRITICAL: All collections depend on this
- **Kernel** (117 spec files) — 90+ global methods missing (puts, print, raise, loop, etc.)

**P1 - Core Types** (needed for basic Ruby):
- Numeric (46 files), Proc (25 files), Method (27 files), Binding (11 files)
- Time (66 files), Struct (32 files), Enumerator (30 files)

**P2 - System** (important but not blocking):
- IO (80 files), File (68 files), Dir (35 files), Exception (39 files)
- Complex (43 files), Rational (34 files), Set (52 files)

**Implementation Completeness Summary**:
- Core Classes: 16/59 (27%)
- String Methods: 60/~114 (53%)
- Array Methods: 55/~102 (54%)
- Hash Methods: 38/~69 (55%)
- Integer Methods: 24/~67 (36%)
- Float Methods: 11/~50 (22%)
- Kernel Methods: ~29/~117 (24%)

### VM and Compiler Capabilities (bg_aae37f41)

**Opcode Implementation Status**:
- Total Defined: 80 opcodes
- Implemented in VM: ~45 opcodes (56%)

**Category Breakdown**:

| Category | Defined | Implemented | % |
|----------|---------|-------------|---|
| Constants/Literals | 6 | 6 | 100% ✅ |
| Arithmetic | 11 | 10 | 91% ✅ |
| Comparison/Logic | 7 | 7 | 100% ✅ |
| Control Flow (Jumps) | 5 | 4 | 80% ✅ |
| Data Structures | 4 | 4 | 100% ✅ |
| Variable Access | 12 | 8 | 67% ⚠️ |
| Functions/Methods | 9 | 4 | 44% ⚠️ |
| Classes/Modules | 8 | 2 | 25% ❌ |
| Blocks/Closures | 4 | 2 | 50% ❌ |
| Exception Handling | 6 | 0 | 0% ❌ |
| Type Introspection | 7 | 0 | 0% ❌ |

**CRITICAL INFRASTRUCTURE GAPS** (Blocks Ruby Core Implementation):

1. **Exception Handling** (0% implemented) — BLOCKER
   - OpRaise, OpRescue, OpRescueMatch, OpRetry defined but NOT in VM
   - BeginExpression AST exists but not compiled
   - Blocks: Exception class, error handling in all core methods

2. **Block/Closure System** (50% implemented) — BLOCKER
   - OpBlock, OpBlockWithArg, OpSendWithBlock NOT in VM
   - OpYield, OpYieldWithValue NOT in VM
   - Blocks: Enumerable methods (each, map, select), all iterators

3. **Type Introspection** (0% implemented) — BLOCKER
   - OpIsA, OpKindOf, OpRespondTo NOT in VM
   - Blocks: `is_a?`, `kind_of?`, `respond_to?` methods

4. **Module System** (25% implemented) — BLOCKER
   - OpInclude, OpExtend, OpPrepend NOT in VM
   - Blocks: Mixing in Enumerable, module inclusion

5. **For Loops** (0% compiled)
   - ForExpression AST not compiled
   - Blocks: `for i in arr` syntax

**Unimplemented AST Nodes**:
- ForExpression, RedoExpression, RetryExpression, SuperExpression
- RegexpLiteral, BeginExpression, RaiseExpression, RescueClause
- AliasExpression, UndefExpression, ProcLiteral
- InstanceVarAssign, ClassVarAssign, GlobalVarAssign (partial)

**ASSESSMENT**: ❌ **Cannot implement Ruby core methods with current infrastructure**

**Infrastructure work required FIRST** (estimated 6-9 weeks):
1. Exception handling (1-2 weeks)
2. Block/yield system (2-3 weeks)
3. Type introspection (1 week)
4. Module system (1-2 weeks)
5. For loops (1 week)

## Open Questions

### Metis Review Findings ✅

Metis identified critical gaps that need user input before finalizing the plan:

**CRITICAL QUESTIONS** (User answers received ✅):

1. ✅ **What Ruby version is RGo targeting?**
   - **ANSWER**: Ruby 4.0.1 (released 2025-12-25, normal maintenance)
   - Impact: Latest Ruby features, newest spec requirements, pattern matching, etc.
   
2. ✅ **What's the team size and hours/week?**
   - **ANSWER**: 兼职 (Part-time)
   - Impact: Timeline estimates need 2-4x adjustment (6-9 weeks → 12-36 weeks for part-time)
   
3. ✅ **Is this a learning project or production-ready interpreter?**
   - **ANSWER**: 生产级 (Production-grade)
   - Impact: Quality, stability, and comprehensive testing are critical
   
4. ✅ **What's acceptable performance vs. MRI?**
   - **ANSWER**: 目标比 MRI 快，初期可接受 <10x 慢，先实现功能后优化性能
   - Impact: Initial naive implementation OK, focus on correctness first, optimize later
   
5. ✅ **What's the definition of "done"?**
   - **ANSWER**: 100% spec 通过 - **选项 B: 所有 specs (core + language + library)**
   - Impact: Very ambitious - 4-5 year project for part-time work
   - **CRITICAL REQUIREMENT**: 所有特性用 Go 原生实现，不使用 Ruby，以提升性能
   - Scope: ~3500+ spec files (core ~1500, language ~69, library ~1000+)

**VALIDATION TASKS RECOMMENDED** (Before detailed planning):
1. Opcode audit (2-4 hours) - verify "45/80 implemented" accuracy
2. Block infrastructure test (1-2 hours) - verify "50% complete" claim
3. MSpec adapter prototype (4-8 hours) - prove test strategy feasibility
4. Exception handling architecture review (2-4 hours) - estimate effort (2 weeks vs. 12 weeks)
5. Ruby version confirmation (30 min) - check docs/code for target version

**IMPORTANT QUESTIONS** (User answers received ✅):

6. ✅ **Do you have CI set up?**
   - **ANSWER**: 现在没有 (Not yet)
   - Impact: Need to add CI setup as Phase 0 task (GitHub Actions recommended)
   
7. ✅ **Can you run Ruby code in RGo today?**
   - **ANSWER**: 本地安装有 Ruby (Local Ruby installation available)
   - Impact: Can test Ruby behavior, run MSpec, prototype adapter
   
8. ✅ **What Ruby features are explicitly out of scope?**
   - **ANSWER**: C 扩展不支持 (例如 sqlite 驱动，用 Go 驱动替代)
   - **CRITICAL**: 所有功能用 Go 原生实现，不依赖 Ruby 运行时
   - Impact: Need Go implementations for: JSON parser, HTTP client, File I/O, Regexp engine, etc.
   - **IN SCOPE**: Threads, Fiber, Refinements, Pattern matching (Ruby 4.0 features) - all in Go

### Previous Questions (Partially Answered)

1. ✅ **Infrastructure Readiness**: Can we implement Ruby core methods with the current VM, or do we need to fix language features first?
   - **ANSWER**: NO. Must complete infrastructure work first (exception handling, blocks, type introspection, modules)
   
2. **Test Framework**: How will we run Ruby specs against RGo? Need MSpec adapter?
   - **DECISION NEEDED**: Build MSpec adapter or create Go-based spec runner?
   - **METIS RECOMMENDATION**: Prototype MSpec adapter BEFORE planning (validation task #3)
   
3. **Scope**: Should we aim for full Ruby 3.3 compatibility, or a practical subset?
   - **RECOMMENDATION**: Practical subset - focus on core classes (Array, String, Hash, Integer, Float) + Enumerable + Kernel
   - **NEEDS CONFIRMATION**: User must define "done" criteria
   
4. **Prioritization**: Should we complete one class fully (e.g., Array) before moving to others, or implement high-frequency methods across all classes?
   - **RECOMMENDATION**: Hybrid approach - complete infrastructure first, then implement by dependency waves
   - **METIS ADDITION**: Implement in order of Ruby spec frequency, not arbitrary order

## Technical Decisions

### Decision 1: Infrastructure-First Approach ✅

**Rationale**: VM analysis reveals critical gaps that block core method implementation:
- Exception handling: 0% (blocks error handling in all methods)
- Block/closure system: 50% (blocks Enumerable, iterators)
- Type introspection: 0% (blocks is_a?, respond_to?)
- Module system: 25% (blocks Enumerable mixing)

**Decision**: Complete infrastructure work BEFORE implementing Ruby core methods.

### Decision 2: Phased Implementation Strategy ✅

**Phase 1 - Infrastructure** (6-9 weeks):
1. Exception handling (begin/rescue/ensure/raise)
2. Block/yield system (blocks, yield, block_given?)
3. Type introspection (is_a?, respond_to?)
4. Module system (include, extend, prepend)
5. For loops

**Phase 2 - Foundation Classes** (8-12 weeks):
1. Enumerable module (foundation for all collections)
2. Kernel module (global methods)
3. Complete Array/String/Hash methods

**Phase 3 - Extended Core** (12-16 weeks):
1. Proc/Lambda/Method (closures)
2. Range, Symbol, Regexp (complete implementations)
3. Numeric, Time, Struct

**Phase 4 - System Classes** (16-24 weeks):
1. IO, File, Dir
2. Exception hierarchy
3. Thread, Fiber (if needed)

### Decision 3: Test Strategy ✅

**Approach**: Incremental spec compliance
1. Build MSpec adapter to run Ruby specs against RGo
2. Start with language specs (69 files) - verify infrastructure
3. Then core class specs by priority (Kernel → String → Array → Hash)
4. Track pass rate per class

### Decision 4: Scope Definition ✅

**Target**: Practical Ruby subset for real-world use

**IN SCOPE**:
- Core classes: Array, String, Hash, Integer, Float, Symbol, Range, Regexp
- Foundation: Enumerable, Kernel, Object, Module, Class
- Closures: Proc, Lambda, Method
- Basic system: IO, File, Time
- Exception handling

**OUT OF SCOPE** (for now):
- Full standard library (JSON, Net::HTTP, etc.)
- Concurrency (Thread, Fiber, Mutex) - unless critical
- Advanced metaprogramming (TracePoint, ObjectSpace)
- C extension compatibility
- Rails compatibility layer

## Scope Boundaries

**INCLUDE**:
- ✅ Infrastructure completion (exception handling, blocks, modules, type introspection)
- ✅ Core class method implementation (Array, String, Hash, Integer, Float, Symbol, Range, Regexp)
- ✅ Foundation modules (Enumerable, Kernel, Object, Module, Class)
- ✅ Closure support (Proc, Lambda, Method, yield, block_given?)
- ✅ Language feature completion (for loops, super, alias, undef)
- ✅ Ruby spec compliance testing (MSpec adapter)
- ✅ Basic system classes (IO, File, Time, Struct)
- ✅ Exception hierarchy (StandardError, ArgumentError, TypeError, etc.)

**EXCLUDE** (for now):
- ❌ Full standard library (JSON, Net::HTTP, FileUtils, ERB, Digest)
- ❌ Concurrency features (Thread, Fiber, Mutex, Queue) - unless critical for specs
- ❌ Advanced metaprogramming (TracePoint, ObjectSpace, Binding)
- ❌ C extension compatibility
- ❌ Rails compatibility layer
- ❌ Pattern matching (Ruby 3.0+ feature)
- ❌ Encoding system (full Unicode support)
- ❌ Process management (fork, spawn, signals)
