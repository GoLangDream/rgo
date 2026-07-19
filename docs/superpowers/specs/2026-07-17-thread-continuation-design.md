# Ruby Thread 可恢复执行设计

## 目标

以单线程协作调度实现 Ruby `Thread.stop`、`sleep`、`wakeup/run`、`join`、`kill/terminate` 和 `raise` 的可恢复语义；线程暂停后从原指令继续，不能重新执行 block 或重复副作用。

## 架构

VM 负责保存和恢复 continuation，core 只维护 Ruby Thread 状态并通过钩子请求 VM 调度。每个首次运行的 Ruby Thread 获得一个由通道握手控制的休眠 Go coroutine，以原样保存嵌套 Go 调用栈；同时保存该线程的 VM 栈、帧、异常/ensure/rescue 控制栈和恢复结果。暂停时切回调用者的 VM 上下文并让 coroutine 阻塞在通道上；恢复时先换回线程上下文，再解除通道阻塞。

调度保持协作式和串行：通道握手保证任一时刻只执行一个 Ruby Thread，不创建对应的 OS 线程；休眠 coroutine 不轮询、不消耗 CPU。状态机为 `new -> runnable -> running -> sleeping/runnable -> dead`。`wakeup/run` 只把 sleeping 线程变为 runnable；`join/pass` 驱动 runnable 队列。

## 中断与退出

`kill/terminate` 和 `Thread#raise` 保存为待注入中断，在目标线程下一个安全点进入现有异常展开路径，使 `ensure` 必须执行、符合条件的 `rescue` 能观察异常。线程完成后释放 continuation；重复 wakeup 或操作 dead Thread 按 Ruby 的 `ThreadError`/返回值规则处理。

## 验证

按风险递增执行：

1. `Thread.stop` 后 `wakeup/run` 从下一条语句继续且之前副作用只发生一次。
2. `sleep/status/join/pass` 状态转换正确。
3. `kill/terminate/raise` 执行 ensure、rescue 和返回值语义。
4. mutex、flock 等既有阻塞点复用相同 continuation。
5. 聚焦 Go 测试后串行运行相关 RubySpec，再刷新整个 Thread 目录。

所有构建和测试保持 `GOMAXPROCS=1`、Go `-p=1`、单 spec 串行、低优先级和超时限制；不执行 Git 操作。
