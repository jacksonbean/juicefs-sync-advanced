# JuiceFS Sync 结果字段详解

本文档详细分析 `juicefs sync` 命令执行完成后输出结果的每个字段，包括其含义、来源（源存储/目标存储）以及触发条件。

---

## 输出示例

```
Found: 1000, excluded: 50 (100 MB), skipped: 200 (500 MB), copied: 300 (1.5 GB), extra: 10 (50 MB), checked: 300 (1.5 GB), deleted: 5, failed: 0
```

---

## 字段来源总览

| 字段 | 来源 | 说明 |
|------|------|------|
| **Found / Scanned** | 源存储 | 扫描到的总对象数 |
| **Excluded** | 源存储 | 因过滤规则排除的对象 |
| **Skipped** | 源+目标对比 | 目标已存在且无需更新 |
| **Copied** | 同步操作 | 成功复制的对象 |
| **Extra** | 目标存储 | 目标独有的对象 |
| **Checked** | 校验操作 | 校验和验证过的对象 |
| **Deleted** | 目标/源存储 | 删除的对象 |
| **Failed** | 操作失败 | 失败的操作数 |
| **Pending** | 任务队列 | 等待处理的任务 |

---

## 详细字段说明

### 1. Found / Scanned（扫描对象）

```
handled = progress.AddCountBar("Scanned objects", 0)
```

| 属性 | 说明 |
|------|------|
| **来源** | 源存储 (Source) |
| **含义** | sync 命令遍历源存储时发现的所有对象总数 |
| **代码位置** | `pkg/sync/sync.go` - `incrTotal(1)` |
| **触发条件** | 每次从源存储 List 获取一个对象时累加 |

---

### 2. Excluded（排除对象）

```
excluded = progress.AddCountSpinner("Excluded objects")
excludedBytes = progress.AddByteSpinner("Excluded bytes")
```

| 属性 | 说明 |
|------|------|
| **来源** | 源存储 (Source) - 过滤后 |
| **含义** | 根据过滤规则被排除的文件 |
| **代码位置** | `pkg/sync/sync.go` - `filter()` 函数 |
| **触发条件** | 使用以下选项时： |

- `--exclude` / `--include` 模式匹配
- `--min-size` / `--max-size` 大小限制
- `--min-age` / `--max-age` 时间限制
- `--start-time` / `--end-time` 时间范围

---

### 3. Skipped（跳过对象）

```
skipped = progress.AddCountSpinner("Skipped objects")
skippedBytes = progress.AddByteSpinner("Skipped bytes")
```

| 属性 | 说明 |
|------|------|
| **来源** | 源+目标对比 |
| **含义** | 目标已存在且无需复制的对象 |
| **代码位置** | `pkg/sync/sync.go` - `produce()` 函数 |
| **触发条件** | |

| 条件 | 说明 |
|------|------|
| `--update` | 目标 mtime >= 源 mtime 时跳过 |
| 未指定 `--update` | 目标大小 = 源大小时跳过 |
| `--existing` | 目标已存在时不创建新文件 |
| `--ignore-existing` | 跳过已存在的文件 |
| `--check-all` | 校验和相同时跳过 |

---

### 4. Copied（复制对象）

```
copied = progress.AddCountSpinner("Copied objects")
copiedBytes = progress.AddByteSpinner("Copied bytes")
```

| 属性 | 说明 |
|------|------|
| **来源** | 实际同步操作 |
| **含义** | 成功从源复制到目标的对象 |
| **代码位置** | `pkg/sync/sync.go` - `CopyData()` 函数 |
| **触发条件** | 源文件需要同步到目标时 |

---

### 5. Extra（额外对象）

```
extra = progress.AddCountSpinner("Extra objects")
extraBytes = progress.AddByteSpinner("Extra bytes")
```

| 属性 | 说明 |
|------|------|
| **来源** | 目标存储 (Target) |
| **含义** | 目标存储中存在但源存储没有的对象 |
| **代码位置** | `pkg/sync/sync.go` - `handleExtraObject()` 函数 |
| **触发条件** | 使用 `--delete-dst` 时会删除这些对象 |

---

### 6. Checked（校验对象）

```
checked = progress.AddCountSpinner("Checked objects")
checkedBytes = progress.AddByteSpinner("Checked bytes")
```

| 属性 | 说明 |
|------|------|
| **来源** | 校验操作 |
| **含义** | 通过校验和验证的对象 |
| **代码位置** | `pkg/sync/sync.go` - `checkSum()` 函数 |
| **触发条件** | |

| 选项 | 说明 |
|------|------|
| `--check-all` | 校验所有文件 |
| `--check-new` | 校验新复制的文件 |
| `--check-change` | 检查源文件是否在同步过程中改变 |

> **注意**: 只有启用 checksum 验证时才显示此字段

---

### 7. Deleted（删除对象）

```
deleted = progress.AddCountSpinner("Deleted objects")
```

| 属性 | 说明 |
|------|------|
| **来源** | 目标存储 / 源存储 |
| **含义** | 被删除的对象 |
| **代码位置** | `pkg/sync/sync.go` - `deleteObj()` 函数 |
| **触发条件** | |

| 选项 | 说明 |
|------|------|
| `--delete-dst` | 删除目标端的多余文件 |
| `--delete-src` | 同步后删除源端已存在的文件 |

---

### 8. Failed（失败对象）

```
failed = progress.AddCountSpinner("Failed objects")
```

| 属性 | 说明 |
|------|------|
| **来源** | 任何操作失败 |
| **含义** | 复制、删除、校验等操作失败的对象数 |
| **代码位置** | `pkg/sync/sync.go` - `worker()` 函数 |
| **触发条件** | 任何同步操作失败时累加 |

---

### 9. Pending（待处理）

```
pending = progress.AddCountSpinner("Pending objects")
```

| 属性 | 说明 |
|------|------|
| **来源** | 任务队列 |
| **含义** | 等待处理的任务数（队列中的对象） |
| **代码位置** | `pkg/sync/sync.go` - `syncExitFunc()` |

---

### 10. Lost（丢失对象）

```
lost: total - handled - extra
```

| 属性 | 说明 |
|------|------|
| **来源** | 计算得出 |
| **含义** | 处理过程中丢失的对象 |
| **触发条件** | 并发处理异常导致的对象丢失 |

---

## 数据流关系图

```
源存储 List
    ↓
[Found/Scanned] ←────── 总扫描数
    ↓
过滤规则 (exclude/include/size/age)
    ↓
[Excluded] ←────────── 被排除
    ↓
与目标对比
    ├─ 目标不存在 → [Copied] 复制
    ├─ 目标已存在 → [Skipped] 跳过
    │               (或触发复制条件时 [Copied])
    └─ 目标有多余 → [Extra] 额外
    
目标存储 List
    ↓
[Extra] ←──────────── 目标独有的对象

同步后校验 (--check-* 选项)
    ↓
[Checked] ←────────── 已校验

删除操作
    ↓
[Deleted] ←────────── 已删除

任何失败
    ↓
[Failed] ←────────── 失败数
```

---

## 核心数据结构

### Progress 定义（实时进度）

位置: `pkg/utils/progress.go`

```go
type Progress struct {
    // 实时进度条和计数器
}
```

方法:
- `AddCountBar` - 计数进度条
- `AddCountSpinner` - 计数旋转器
- `AddByteSpinner` - 字节旋转器

### Stat 结构（跨进程统计）

位置: `pkg/sync/cluster.go`

```go
type Stat struct {
    Copied       int64
    CopiedBytes  int64
    Checked      int64
    CheckedBytes int64
    Deleted      int64
    Skipped      int64
    SkippedBytes int64
    Failed       int64
    // ...
}
```

---

## 常见场景分析

| 场景 | 预期结果 |
|------|----------|
| **首次同步** | Found=源文件数, Copied≈Found, Skipped=0 |
| **增量同步(无更新)** | Found=源文件数, Skipped≈Found, Copied=0 |
| **增量同步(有更新)** | Found=源文件数, Copied=更新数, Skipped=未更新数 |
| **目标有额外文件** | Extra=额外数 (如使用 --delete-dst 会被删除) |
| **完整校验同步** | Copied后有Checked统计 |

---

## 代码文件索引

| 文件 | 说明 |
|------|------|
| `cmd/sync.go` | sync 命令入口，CLI 参数定义 |
| `pkg/sync/sync.go` | 核心同步引擎，任务分发，进度统计 |
| `pkg/sync/cluster.go` | 分布式统计 (Stat 结构) |
| `pkg/utils/progress.go` | 实时进度 UI 实现 |

---

## 参考资料

- [JuiceFS 官方文档 - sync 命令](https://juicefs.com/docs/community/command_reference#sync)
- [JuiceFS GitHub](https://github.com/juicedata/juicefs)
