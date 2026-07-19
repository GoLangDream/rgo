# Struct 设计

目标：完整实现当前 RubySpec 覆盖的 Ruby `Struct` 语义，使 `vendor/ruby/spec/core/struct` 全绿。

设计：生成类在类实例变量中保存字段顺序及 `keyword_init` 模式；实例仍使用现有 `object.Object`。生成类只定义字段 reader/writer 与初始化入口，通用的索引、枚举、转换、比较、hash、inspect、dig 等行为由 `Struct` 基类统一提供。字段值和普通实例变量分离，避免 `instance_variable_get` 与成员访问互相污染。递归比较、hash 和 inspect 使用访问中的对象集合防止无限递归。

初始化：默认模式按位置赋值，并按 Ruby 3 规则处理 keyword hash；`keyword_init: true` 只接受声明字段的 Hash/keywords；`false` 使用位置语义。参数过多、重复字段、非法字段/常量名和未知 keyword 返回对应 Ruby 异常。`Struct.new` 的 block 使用生成类作为 self 并把生成类作为 block 参数。

验收：30 个 Struct spec、182 examples、0 failures；相关 VM 回归与 Marshal 回归通过。所有构建和测试串行、单核、低优先级；不执行 Git。
