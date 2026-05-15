# juicefs-sync-advanced 项目架构分析文档

> 生成时间：2026-05-15
> 目标文件：docs/architecture.md

---

## 1. 项目整体架构

### 1.1 目录结构

```
juicefs-sync-advanced/
├── cmd/
│   └── sync.go              # CLI 入口，定义所有命令行参数
├── pkg/
│   ├── sync/
│   │   ├── sync.go          # 核心同步逻辑（含 produce、worker、Sync 主函数）
│   │   ├── config.go        # Config 结构体定义与 CLI 参数解析
│   │   ├── cluster.go       # 多节点分布式同步（manager/worker 模式）
│   │   └── download.go      # 并行下载器实现
│   ├── object/              # 存储后端抽象层（支持 20+ 种存储）
│   ├── metric/              # Prometheus 监控指标
│   ├── utils/               # 工具函数（日志、进度条等）
│   └── version/             # 版本信息
└── go.mod
```

### 1.2 cmd/pkg 分层

```
┌─────────────────────────────────────────────────────────┐
│                        cmd/sync.go                       │
│              (CLI 入口：urfave/cli 框架)                  │
│  - 定义 syncActionFlags() 中的 --list-extra / --list-lost │
│  - 调用 createSyncStorage() 解析 SRC/DST URI            │
│  - 调用 sync.NewConfigFromCli() 构建 Config              │
│  - 调用 sync.Sync() 启动同步                             │
└──────────────────────┬──────────────────────────────────┘
                       │ *Config
                       ▼
┌─────────────────────────────────────────────────────────┐
│                        pkg/sync/                         │
│                                                          │
│  ┌─────────────────┐    ┌────────────────────────────┐   │
│  │   config.go     │    │        sync.go             │   │
│  │                 │    │                            │   │
│  │ Config 结构体   │───▶│ produce()                  │   │
│  │                 │    │   ├── filter()             │   │
│  │ NewConfigFromCli│    │   ├── handleExtraObject()  │   │
│  │ ListExtra/Lost  │    │   └── skipIt()              │   │
│  │                 │    │                            │   │
│  └─────────────────┘    │ worker()                   │   │
│                         │   ├── CopyData()            │   │
│                         │   ├── deleteObj()          │   │
│                         │   └── checkSum()           │   │
│                         │                            │   │
│                         │ Sync()                     │   │
│                         │   ├── startProducer()      │   │
│                         │   └── startSingleProducer() │   │
│                         └────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### 1.3 Config 结构体（config.go:34-102）

```go
type Config struct {
    // 同步行为控制
    DeleteSrc    bool   // --delete-src
    DeleteDst    bool   // --delete-dst
    Update       bool   // --update
    ForceUpdate  bool   // --force-update
    Dry          bool   // --dry

    // 过滤条件
    Start/End    string // --start / --end
    MaxSize/MinSize int64
    MaxAge/MinAge time.Duration
    StartTime/EndTime time.Time

    // 列表报告（本文档重点）
    ListExtra    bool   // --list-extra（目标存在、源不存在）
    ListLost     bool   // --list-lost（源存在、目标不存在）

    // 其他
    Src/Dst      string // 源/目标 URI（脱敏后）
    State        *SyncState // 同步状态（进度条等）
    // ... 省略若干字段
}
```

---

## 2. 同步流程核心链路

### 2.1 从启动到结束的函数调用链

```
main()
  └─ cmd/sync.go:doSync()                    [CLI 入口]
       ├─ createSyncStorage(srcURL)           [解析源存储]
       ├─ createSyncStorage(dstURL)          [解析目标存储]
       └─ sync.Sync(ctx, src, dst, config)   [启动同步]
            │
            ▼
         pkg/sync/sync.go:1991:Sync()
            │
            ├─ 初始化 config.State = NewSyncState(threads)
            │
            ├─ 初始化进度条（state.InitBars）
            │
            ├─ 启动 N 个 worker goroutine
            │   worker(tasks, src, dst, config)     [~sync.go:1070]
            │   └── 处理 tasks 队列中的 object
            │       ├── CopyData() → doCopySingle/doCopyMultiple
            │       ├── deleteObj() → 标记为 markDeleteDst/markDeleteSrc
            │       ├── checkSum() → markChecksum
            │       └── copyPerms() → markCopyPerms
            │
            ├─ 启动生产者（根据 FilesFrom 选择）
            │   │
            │   ├─ produceFromList()  [files-from 模式，~sync.go:1803]
            │   │   └── startProducer() → startSingleProducer() → produce()
            │   │
            │   └─ startProducer()    [普通模式，~sync.go:1897]
            │       └── startSingleProducer()    [~sync.go:1360]
            │           └─ produce()              [~sync.go:1383]
            │
            ├─ close(tasks)  [生产者完成后关闭任务通道]
            │
            └─ wg.Wait()     [等待所有 worker 完成]
                │
                ├─ 延迟删除目录（dstDelayDel/srcDelayDel）
                ├─ 输出统计信息（syncExitFunc）
                └─ 写入 record 回调（OnSummary）
```

### 2.2 核心函数说明

| 函数 | 文件位置 | 职责 |
|------|---------|------|
| `doSync()` | cmd/sync.go:485 | CLI 入口，解析参数，创建存储，调用 `sync.Sync()` |
| `Sync()` | pkg/sync/sync.go:1991 | 同步主函数，初始化状态，启动 worker 和 producer |
| `startProducer()` | pkg/sync/sync.go:1897 | 多线程列表入口（支持 --list-depth） |
| `startSingleProducer()` | pkg/sync/sync.go:1360 | 单线程列表入口，调用 `ListAll()` 和 `produce()` |
| `produce()` | pkg/sync/sync.go:1383 | **核心决策逻辑**：决定每个 key 是 copy/skip/delete/extra/lost |
| `worker()` | pkg/sync/sync.go:1070 | 处理实际的文件操作（复制/删除/校验/权限） |
| `handleExtraObject()` | pkg/sync/sync.go:1331 | 处理目标多余对象（--delete-dst 或 --list-extra） |
| `ListAll()` | pkg/sync/sync.go:291 | 遍历存储中的所有对象 |
| `CopyData()` | pkg/sync/sync.go:998 | 复制文件数据（支持单次和分段上传） |

---

## 3. list-extra 和 list-lost 的具体实现

### 3.1 配置绑定

```go
// cmd/sync.go:213-218（flag 定义）
&cli.BoolFlag{
    Name:  "list-extra",
    Usage: "print extra keys (present in destination but not in source) to stderr",
},
&cli.BoolFlag{
    Name:  "list-lost",
    Usage: "print lost keys (present in source but not in destination) to stderr",
},

// config.go:100-101（Config 字段）
ListExtra bool  // --list-extra
ListLost  bool  // --list-lost

// config.go:249-250（从 CLI 解析）
ListExtra: c.Bool("list-extra"),
ListLost:  c.Bool("list-lost"),
```

### 3.2 输出逻辑详解

#### 3.2.1 --list-extra（目标存在、源不存在）

**触发位置**：`pkg/sync/sync.go:1331-1344`

```go
func handleExtraObject(tasks chan<- object.Object, dstobj object.Object, config *Config) bool {
    state := getState(config)
    state.IncrTotal(1)
    // 条件：不删除 || 不同步目录 || limit 为 0（不执行任何操作）
    if !config.DeleteDst || !config.Dirs && dstobj.IsDir() || config.Limit == 0 {
        logger.Debug("Ignore extra object", dstobj.Key())
        state.extra.Increment()
        state.extraBytes.IncrInt64(dstobj.Size())
        if config.ListExtra {
            logger.Infof("[Extra] %s", dstobj.Key())  // ← 输出点
        }
        // ... dry-run 回调
        return false
    }
    // 否则，加入 tasks 队列准备删除
    // ...
}
```

**触发时机**：
- 当源遍历完毕，目标仍有剩余对象时
- `produce()` 函数（约 sync.go:1426-1443）在以下场景调用：
  1. `dstobj.Key() > obj.Key()` 且源已没有更多匹配对象
  2. `dstobj.Key() > obj.Key()` 且当前源对象小于目标对象

**输出格式**：`[Extra] <key>` 写入 stderr（通过 `logger.Infof`）

---

#### 3.2.2 --list-lost（源存在、目标不存在）

**触发位置**：`pkg/sync/sync.go:1399-1409`

```go
skipIt := func(obj object.Object) {
    skip++
    skipBytes += obj.Size()
    if config.ListLost {
        logger.Infof("[Lost] %s", obj.Key())  // ← 输出点
    }
    if skip > 100 || time.Since(lastUpdate) > time.Millisecond*100 {
        lastUpdate = time.Now()
        flushProgress()
    }
}
```

**触发时机**：`produce()` 函数中判断源对象不需要复制时，调用 `skipIt()`：
- `config.Existing` 且目标不存在（约 sync.go:1449-1454）
- `config.IgnoreExisting` 且目标已存在（约 sync.go:1461-1467）
- `config.Update` 且源比目标旧（约 sync.go:1484-1488）
- 其他情况认为"相同"而跳过（约 sync.go:1510-1514）

**输出格式**：`[Lost] <key>` 写入 stderr（通过 `logger.Infof`）

### 3.3 代码路径汇总

| 步骤 | 文件:行号 | 说明 |
|------|----------|------|
| 1. Flag 定义 | `cmd/sync.go:213-218` | 注册 `--list-extra` 和 `--list-lost` |
| 2. Config 解析 | `pkg/sync/config.go:249-250` | 将 CLI 布尔值写入 Config.ListExtra/ListLost |
| 3. Extra 检测 | `pkg/sync/sync.go:1331-1344` | `handleExtraObject()` 检测目标多余对象并输出 |
| 4. Lost 检测 | `pkg/sync/sync.go:1402-1403` | `skipIt()` 匿名函数检测源缺失对象并输出 |
| 5. 日志输出 | `pkg/utils/logger.go` | `logger.Infof()` 将消息写入 stderr（`logrus` 库） |

### 3.4 输出重定向关系

```
logger.Infof("[Extra] %s", key)
       │
       ▼
logrus.InfoLevel
       │
       ▼
logHandle.SetOutput(w io.Writer)  [默认 stderr]
       │
       ▼
输出格式: "2026/05/15 23:32:02.000000 juicefs[I]<INFO>: [Extra] <key> [函数名@文件名:行号]\n"
```

**日志配置**（pkg/utils/logger.go）：
- 默认输出到 `os.Stderr`
- 使用 `logrus` 库，支持格式化时间戳和调用栈
- 可通过 `SetOutput()` 或 `SetOutFile()` 重定向

### 3.5 典型场景示例

**场景 1**：源 `{a, b, c}`，目标 `{a, b, d}`，执行 `--list-extra`

```
SRC:  a, b, c
DST:  a, b, d

produce() 遍历：
1. obj=a, dstobj=a → 相等，不 extra
2. obj=b, dstobj=b → 相等，不 extra
3. obj=c, dstobj=d → c < d，c 加入 tasks
4. 源遍历完毕，目标剩余 d
5. handleExtraObject(d) 被调用
   → config.ListExtra = true
   → logger.Infof("[Extra] %s", "d")  →  输出: [Extra] d
```

**场景 2**：源 `{a, b, c}`，目标 `{a, b}`，执行 `--list-lost --existing`

```
SRC:  a, b, c
DST:  a, b

produce() 遍历：
1. obj=a, dstobj=a → 相等，不 lost
2. obj=b, dstobj=b → 相等，不 lost
3. obj=c, dstobj=nil → c > b，目标不存在
   → config.Existing = true
   → skipIt(c) 被调用
     → logger.Infof("[Lost] %s", "c")  →  输出: [Lost] c
```

---

## 4. 关键代码索引

### 4.1 入口与配置

- `cmd/sync.go:485` — `doSync()` 函数入口
- `cmd/sync.go:213-218` — `--list-extra` / `--list-lost` flag 定义
- `pkg/sync/config.go:34-102` — `Config` 结构体
- `pkg/sync/config.go:249-250` — `ListExtra` / `ListLost` 解析

### 4.2 核心同步逻辑

- `pkg/sync/sync.go:1991` — `Sync()` 主函数
- `pkg/sync/sync.go:1383` — `produce()` 核心决策函数
- `pkg/sync/sync.go:1070` — `worker()` 文件操作执行器
- `pkg/sync/sync.go:1360` — `startSingleProducer()` 列表生产者
- `pkg/sync/sync.go:1331` — `handleExtraObject()` 多余对象处理

### 4.3 list-extra / list-lost 输出

- `pkg/sync/sync.go:1338-1339` — `ListExtra` 输出 `logger.Infof("[Extra] %s", ...)`
- `pkg/sync/sync.go:1402-1403` — `ListLost` 输出 `logger.Infof("[Lost] %s", ...)`
- `pkg/sync/sync.go:1399-1409` — `skipIt()` 匿名函数定义

### 4.4 日志系统

- `pkg/utils/logger.go:134` — `GetLogger()` 获取 logger 实例
- `pkg/utils/logger.go:176` — `SetOutput()` 设置输出目标

---

## 5. 总结

1. **架构分层清晰**：`cmd` 层负责 CLI 解析和存储初始化，`pkg/sync` 层负责核心同步逻辑。
2. **--list-extra**：在 `handleExtraObject()` 中触发，目标对象在源遍历完后仍有剩余时输出。
3. **--list-lost**：在 `skipIt()` 匿名函数中触发，源对象被判定为"跳过"时输出（目标不存在或不需要复制）。
4. **输出目标**：均通过 `logger.Infof()` 写入 stderr，采用 `logrus` 库进行格式化。
5. **格式**：`[Extra] <key>` 和 `[Lost] <key>`，每行一个 key。