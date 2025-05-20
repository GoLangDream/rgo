# RClass - Ruby风格的类系统

`RClass` 是一个 Go 语言实现的模拟 Ruby 类系统的类型。它提供了丰富的类操作方法，可以让 Go 开发者使用类似 Ruby 的类 API。

## 特性

- 类定义和方法定义
- 实例方法和类方法
- 属性访问器（读写、只读、只写）
- 实例变量和类变量
- 继承和方法重写
- 父类方法调用（Super）
- 方法缺失处理
- 类型检查
- 线程安全

## 创建类

```go
// 创建一个新的类
Person := rgo.Class("Person")
```

## 定义方法

### 实例方法

```go
Person := rgo.Class("Person").
    Define("say_hello", func(self *rgo.RClass) string {
        return "Hello!"
    })
```

### 类方法

```go
Person := rgo.Class("Person").
    DefineClass("create", func(name string) *rgo.RClass {
        p := rgo.Class("Person").New()
        p.SetInstanceVar("name", name)
        return p
    })
```

## 属性访问器

### 读写属性

```go
Person := rgo.Class("Person").
    AttrAccessor("name", "age")  // 生成 name, name=, age, age= 方法
```

### 只读属性

```go
Person := rgo.Class("Person").
    AttrReader("id")  // 只生成 id 方法
```

### 只写属性

```go
Person := rgo.Class("Person").
    AttrWriter("password")  // 只生成 password= 方法
```

## 实例变量

```go
// 设置实例变量
person.SetInstanceVar("name", "John")

// 获取实例变量
name := person.GetInstanceVar("name").(string)
```

## 类变量

```go
// 设置类变量
Person.SetClassVar("version", "1.0.0")

// 获取类变量
version := Person.GetClassVar("version").(string)
```

## 继承

```go
// 创建父类
Animal := rgo.Class("Animal").
    Define("speak", func(self *rgo.RClass) string {
        return "Some sound"
    })

// 创建子类
Dog := rgo.Class("Dog").
    Inherit(Animal).
    Define("speak", func(self *rgo.RClass) string {
        return "Woof!"
    })
```

## 方法缺失处理

```go
Dynamic := rgo.Class("Dynamic").
    MethodMissing(func(name string, args ...any) any {
        return fmt.Sprintf("Called %s with args: %v", name, args)
    })
```

## 调用父类方法

```go
Dog := rgo.Class("Dog").
    Inherit(Animal).
    Define("speak", func(self *rgo.RClass) string {
        // 调用父类的 speak 方法
        parentSound := self.Super("speak").(string)
        return parentSound + " But I'm a dog!"
    })
```

## 类型检查

```go
// 检查是否为指定类的实例
if dog.IsA("Dog") {
    // 是 Dog 类的实例
}

if dog.IsA("Animal") {
    // 是 Animal 类的实例
}
```

## 方法查询

```go
// 获取所有实例方法
methods := person.Methods()

// 获取所有类方法
classMethods := Person.ClassMethods()
```

## 变量查询

```go
// 获取所有实例变量
instanceVars := person.InstanceVars()

// 获取所有类变量
classVars := Person.ClassVars()
```

## 完整示例

### 基本类定义

```go
// 创建一个 Person 类
Person := rgo.Class("Person").
    AttrAccessor("name", "age").
    Define("initialize", func(name string, age int) *rgo.RClass {
        p := rgo.Class("Person").New()
        p.SetInstanceVar("name", name)
        p.SetInstanceVar("age", age)
        return p
    }).
    Define("introduce", func(self *rgo.RClass) string {
        name := self.GetInstanceVar("name").(string)
        age := self.GetInstanceVar("age").(int)
        return fmt.Sprintf("Hi, I'm %s and I'm %d years old.", name, age)
    })

// 创建实例
person := Person.Call("initialize", "John", 30).(*rgo.RClass)

// 使用属性访问器
fmt.Println(person.Call("name")) // 输出: John
person.Call("name=", "Johnny")
fmt.Println(person.Call("name")) // 输出: Johnny

// 调用方法
fmt.Println(person.Call("introduce")) // 输出: Hi, I'm Johnny and I'm 30 years old.
```

### 继承示例

```go
// 创建一个 Animal 类
Animal := rgo.Class("Animal").
    AttrAccessor("name").
    Define("initialize", func(name string) *rgo.RClass {
        a := rgo.Class("Animal").New()
        a.SetInstanceVar("name", name)
        return a
    }).
    Define("speak", func(self *rgo.RClass) string {
        return "Some sound"
    })

// 创建一个 Dog 类，继承自 Animal
Dog := rgo.Class("Dog").
    Inherit(Animal).
    Define("speak", func(self *rgo.RClass) string {
        return "Woof!"
    })

// 创建一个 Cat 类，继承自 Animal
Cat := rgo.Class("Cat").
    Inherit(Animal).
    Define("speak", func(self *rgo.RClass) string {
        return "Meow!"
    })

// 创建实例
dog := Dog.Call("initialize", "Rex").(*rgo.RClass)
cat := Cat.Call("initialize", "Whiskers").(*rgo.RClass)

// 调用方法
fmt.Println(dog.Call("speak")) // 输出: Woof!
fmt.Println(cat.Call("speak")) // 输出: Meow!
```

### 类方法示例

```go
// 创建一个 Math 类
Math := rgo.Class("Math").
    DefineClass("add", func(a, b int) int {
        return a + b
    }).
    DefineClass("subtract", func(a, b int) int {
        return a - b
    }).
    DefineClass("multiply", func(a, b int) int {
        return a * b
    }).
    DefineClass("divide", func(a, b int) float64 {
        return float64(a) / float64(b)
    })

// 调用类方法
sum := Math.Call("add", 2, 3).(int)           // 返回 5
diff := Math.Call("subtract", 5, 3).(int)     // 返回 2
product := Math.Call("multiply", 4, 2).(int)  // 返回 8
quotient := Math.Call("divide", 10, 2).(float64) // 返回 5.0
```

### 方法缺失处理示例

```go
// 创建一个动态类
Dynamic := rgo.Class("Dynamic").
    MethodMissing(func(name string, args ...any) any {
        return fmt.Sprintf("Called %s with args: %v", name, args)
    })

// 创建实例
d := Dynamic.New()

// 调用未定义的方法
fmt.Println(d.Call("undefined_method", "arg1", "arg2"))
// 输出: Called undefined_method with args: [arg1 arg2]
```

### 综合示例：银行账户系统

```go
// 创建一个银行账户类
Account := rgo.Class("Account").
    AttrAccessor("account_number", "balance").
    Define("initialize", func(account_number string, initial_balance float64) *rgo.RClass {
        account := rgo.Class("Account").New()
        account.SetInstanceVar("account_number", account_number)
        account.SetInstanceVar("balance", initial_balance)
        return account
    }).
    Define("deposit", func(self *rgo.RClass, amount float64) float64 {
        current_balance := self.GetInstanceVar("balance").(float64)
        new_balance := current_balance + amount
        self.SetInstanceVar("balance", new_balance)
        return new_balance
    }).
    Define("withdraw", func(self *rgo.RClass, amount float64) (float64, error) {
        current_balance := self.GetInstanceVar("balance").(float64)
        if current_balance < amount {
            return current_balance, fmt.Errorf("余额不足")
        }
        new_balance := current_balance - amount
        self.SetInstanceVar("balance", new_balance)
        return new_balance, nil
    }).
    Define("transfer", func(self *rgo.RClass, target *rgo.RClass, amount float64) error {
        _, err := self.Call("withdraw", amount)
        if err != nil {
            return err
        }
        target.Call("deposit", amount)
        return nil
    }).
    DefineClass("create_savings", func(account_number string) *rgo.RClass {
        return Account.Call("initialize", account_number, 0.0).(*rgo.RClass)
    })

// 创建一个储蓄账户类，继承自 Account
SavingsAccount := rgo.Class("SavingsAccount").
    Inherit(Account).
    Define("initialize", func(account_number string, initial_balance float64, interest_rate float64) *rgo.RClass {
        account := Account.Call("initialize", account_number, initial_balance).(*rgo.RClass)
        account.SetInstanceVar("interest_rate", interest_rate)
        return account
    }).
    Define("add_interest", func(self *rgo.RClass) float64 {
        balance := self.GetInstanceVar("balance").(float64)
        interest_rate := self.GetInstanceVar("interest_rate").(float64)
        interest := balance * interest_rate
        return self.Call("deposit", interest).(float64)
    })

// 使用示例
func main() {
    // 创建普通账户
    account1 := Account.Call("initialize", "A001", 1000.0).(*rgo.RClass)
    account2 := Account.Call("initialize", "A002", 500.0).(*rgo.RClass)

    // 存款
    new_balance := account1.Call("deposit", 500.0).(float64)
    fmt.Printf("存款后余额: %.2f\n", new_balance) // 输出: 存款后余额: 1500.00

    // 取款
    balance, err := account1.Call("withdraw", 200.0).(float64, error)
    if err != nil {
        fmt.Println("取款失败:", err)
    } else {
        fmt.Printf("取款后余额: %.2f\n", balance) // 输出: 取款后余额: 1300.00
    }

    // 转账
    err = account1.Call("transfer", account2, 300.0).(error)
    if err != nil {
        fmt.Println("转账失败:", err)
    } else {
        fmt.Printf("账户1余额: %.2f\n", account1.Call("balance").(float64)) // 输出: 账户1余额: 1000.00
        fmt.Printf("账户2余额: %.2f\n", account2.Call("balance").(float64)) // 输出: 账户2余额: 800.00
    }

    // 创建储蓄账户
    savings := SavingsAccount.Call("initialize", "S001", 1000.0, 0.05).(*rgo.RClass)

    // 添加利息
    new_balance = savings.Call("add_interest").(float64)
    fmt.Printf("添加利息后余额: %.2f\n", new_balance) // 输出: 添加利息后余额: 1050.00

    // 使用类方法创建储蓄账户
    new_savings := Account.Call("create_savings", "S002").(*rgo.RClass)
    fmt.Printf("新储蓄账户余额: %.2f\n", new_savings.Call("balance").(float64)) // 输出: 新储蓄账户余额: 0.00
}
```

## 注意事项

1. 所有方法都返回 `*RClass` 以支持链式调用
2. 实例方法的第一个参数总是 `self *rgo.RClass`
3. 类变量在所有实例间共享
4. 实例变量是每个实例独立的
5. 继承时，子类可以重写父类的方法
6. 方法调用使用 `Call` 方法，需要类型断言获取返回值
7. 属性访问器会自动生成 getter 和 setter 方法
8. 方法缺失处理器可以处理未定义的方法调用
9. 类方法只能通过类调用，不能通过实例调用
10. 实例方法只能通过实例调用，不能通过类调用
11. 父类的方法可以被重写，但可以通过 `Super()` 调用
12. 类型检查支持继承关系
13. 所有方法都是线程安全的
