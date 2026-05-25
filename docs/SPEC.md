# JuiceFS Sync Advanced 文档

**版本**: v1.0.0 | **构建**: 2026-02-05 | **许可证**: Apache License 2.0

---

## 目录

1. [概述](#1-概述)
2. [架构设计](#2-架构设计)
3. [核心模块](#3-核心模块)
4. [功能特性](#4-功能特性)
5. [部署与发布](#5-部署与发布)
6. [适用场景](#6-适用场景)
7. [存储后端支持](#7-存储后端支持)
8. [性能调优](#8-性能调优)
9. [监控与运维](#9-监控与运维)
10. [常见问题](#10-常见问题)

---

## 1. 概述

**JuiceFS Sync Advanced** 是基于 JuiceFS sync 命令的增强版对象存储同步工具，专注于任务持久化、断点续传、智能过滤和 Web UI 调度。

### 核心定位

- 替代 JuiceFS 原生 `juicefs sync` 命令，提供更完善的任务管理和监控能力
- 单二进制部署，无需额外依赖
- 同时支持 CLI 和 Web UI 两种交互方式

---

## 2. 架构设计

### 2.1 系统架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                         JuiceFS Sync Advanced                        │
├─────────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │
│  │   Web UI     │    │   REST API   │    │   Scheduler │          │
│  │  (React)     │    │   Server     │    │  (Cron)      │          │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘          │
│         │                   │                   │                   │
│         └───────────────────┼───────────────────┘                   │
│                             │                                       │
│  ┌──────────────────────────▼──────────────────────────────────┐  │
│  │                     SQLite Database                           │  │
│  │  ┌─────────────────┐    ┌─────────────────────────────┐   │  │
│  │  │  History DB      │    │  Schedule DB                 │   │  │
│  │  │  (任务历史记录)   │    │  (定时任务配置)               │   │  │
│  │  └─────────────────┘    └─────────────────────────────┘   │  │
│  └─────────────────────────────────────────────────────────────┘  │
│                             │                                       │
│  ┌──────────────────────────▼──────────────────────────────────┐  │
│  │                      Sync Engine                             │  │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────────────────┐ │  │
│  │  │  Scanner   │──│  Filter    │──│  Worker Pool          │ │  │
│  │  │  (对象列举)│  │  (规则过滤) │  │  (并发任务执行)        │ │  │
│  │  └────────────┘  └────────────┘  └────────────────────────┘ │  │
│  │                             │                                 │  │
│  │  ┌──────────────────────────▼──────────────────────────────┐│  │
│  │  │              Object Storage Layer                       ││  │
│  │  │  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ││  │
│  │  │  │  S3    │ │  OSS   │ │  OBS   │ │  COS   │ │  ...   │ ││  │
│  │  │  └────────┘ └────────┘ └────────┘ └────────┘ └────────┘ ││  │
│  │  └──────────────────────────────────────────────────────────┘│  │
│  └───────────────────────────────────────────────────────────────┘  │
│                             │                                       │
│  ┌──────────────────────────▼──────────────────────────────────┐  │
│  │                      Metrics Export                            │  │
│  │  Prometheus / Consul                                           │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 核心设计原则

| 设计原则 | 说明 |
|---------|------|
| **无状态 Worker** | Sync 任务本身无状态，状态通过 Config 传入 |
| **全局状态封装** | 使用 `SyncState` + `atomic.Pointer` 避免竞态 |
| **接口抽象** | `ObjectStorage` 接口统一抽象 30+ 存储后端 |
| **渐进式增强** | 从 JuiceFS sync 核心逐步添加增强功能 |

---

## 3. 核心模块

### 3.1 模块结构

```
juicefs-sync-advanced/
├── main.go              # 程序入口
├── webdist.go          # Web 资源内嵌 (Go embed)
├── cmd/                 # CLI 命令层
│   ├── main.go          # CLI 主入口 (urfave/cli)
│   ├── sync.go         # sync 命令实现
│   ├── scan.go         # scan 命令实现
│   ├── ui.go           # UI 启动命令
│   ├── flags.go        # CLI 参数定义
│   ├── metrics_server.go  # 指标服务
│   └── ...
├── pkg/
│   ├── api/            # REST API 服务层
│   │   ├── server.go   # HTTP Server
│   │   ├── handlers.go # 请求处理
│   │   ├── db.go       # 数据库操作
│   │   └── scheduler.go # Cron 调度器
│   ├── sync/           # 核心同步引擎
│   │   ├── sync.go     # 同步主逻辑
│   │   ├── config.go   # 同步配置
│   │   ├── cluster.go  # 集群模式
│   │   └── download.go # 分片下载器
│   ├── object/         # 存储后端层
│   │   ├── interface.go  # 存储接口定义
│   │   ├── s3.go      # AWS S3
│   │   ├── oss.go     # 阿里云 OSS
│   │   ├── obs.go     # 华为云 OBS
│   │   ├── cos.go     # 腾讯云 COS
│   │   ├── file.go    # 本地文件系统
│   │   └── ... (30+ 后端)
│   ├── scan/          # 对象扫描
│   ├── metric/        # 指标收集
│   └── utils/         # 工具函数
└── web/               # React 前端
```

### 3.2 SyncState 状态管理

```go
// pkg/sync/sync.go:63
type SyncState struct {
    handled, pending     *utils.Bar   // 已扫描对象
    copied, copiedBytes  *utils.Bar   // 已复制对象
    checked, checkedBytes *utils.Bar // 已校验对象
    skipped, skippedBytes *utils.Bar // 已跳过对象
    excluded, excludedBytes *utils.Bar // 已排除对象
    extra, extraBytes    *utils.Bar   // 目标端多余对象
    deleted, failed     *utils.Bar   // 已删除/失败对象
    listedPrefix        *utils.Bar   // 已列举前缀
    concurrent          chan int     // 并发控制通道
    limiter             *mixedLimiter // 速率限制器
    totalHandled        atomic.Int64  // 累计处理数
}
```

### 3.3 ObjectStorage 接口

```go
// pkg/object/interface.go:82
type ObjectStorage interface {
    String() string
    Limits() Limits
    Create(ctx context.Context) error
    Get(ctx context.Context, key string, off, limit int64) (io.ReadCloser, error)
    Put(ctx context.Context, key string, in io.Reader) error
    Copy(ctx context.Context, dst, src string) error
    Delete(ctx context.Context, key string) error
    Head(ctx context.Context, key string) (Object, error)
    List(ctx context.Context, prefix, marker, delimiter string, limit int64) ([]Object, bool, string, error)
    ListAll(ctx context.Context, prefix, marker string) (<-chan Object, error)
    CreateMultipartUpload(ctx context.Context, key string) (*MultipartUpload, error)
    UploadPart(ctx context.Context, key, uploadID string, num int, body []byte) (*Part, error)
    CompleteUpload(ctx context.Context, key, uploadID string, parts []*Part) error
    // ...
}
```

---

## 4. 功能特性

### 4.1 核心同步功能

| 功能 | 说明 | CLI Flag |
|------|------|---------|
| **基本同步** | 源到目标的全量同步 | `sync SRC DST` |
| **增量同步** | 基于 mtime 只同步变化文件 | `--update` |
| **强制更新** | 忽略 mtime 强制覆盖 | `--force-update` |
| **并发控制** | 调整并发线程数 | `-p, --threads` |
| **断点续传** | MPU 大文件分片上传中断恢复 | 自动 |

### 4.2 过滤规则

| 功能 | 说明 | CLI Flag |
|------|------|---------|
| **文件名过滤** | include/exclude 模式匹配 | `--include`, `--exclude` |
| **正则过滤** | 支持正则表达式 | `--include-regex`, `--exclude-regex` |
| **大小过滤** | 按文件大小范围筛选 | `--min-size`, `--max-size` |
| **时间过滤** | 按修改时间筛选 | `--min-age`, `--max-age`, `--start-time`, `--end-time` |
| **范围限定** | 限定 key 范围 | `--start`, `--end` |
| **全路径匹配** | 按完整路径匹配 | `--match-full-path` |

### 4.3 任务管理

| 功能 | 说明 | CLI Flag |
|------|------|---------|
| **任务持久化** | SQLite 记录任务状态 | `--record-db-type`, `--record-db-dsn` |
| **定时调度** | Cron 表达式定时执行 | `--schedule "0 2 * * *"` |
| **配置模板** | 保存/复用同步配置 | Web UI |
| **历史回看** | 查看历史同步记录 | Web UI |
| **失败重试** | 失败对象重试 | Web UI |

### 4.4 数据校验

| 功能 | 说明 | CLI Flag |
|------|------|---------|
| **完整校验** | 同步后校验所有文件 | `--check-all` |
| **新增校验** | 只校验新复制文件 | `--check-new` |
| **变化检测** | 检测源文件是否变化 | `--check-change` |
| **校验和校验** | CRC32 分片校验 | 自动 |

### 4.5 错误重试

| 功能 | 说明 | CLI Flag |
|------|------|---------|
| **可配置重试** | 失败操作重试次数可调，默认 5 次 | `--retry-times` |
| **指数退避** | 重试间隔递增（1s, 4s, 9s...） | 自动 |
| **范围限制** | 支持 1-100 次重试次数 | `--retry-times 1-100` |
| **全局生效** | Delete/Get/Put/UploadPart 等全部 I/O 操作生效 | 自动 |

### 4.6 特殊操作

| 功能 | 说明 | CLI Flag |
|------|------|---------|
| **删除源端** | 同步后删除源文件 | `--delete-src` |
| **删除目标端** | 删除目标端多余文件 | `--delete-dst` |
| **保留权限** | 同步文件权限 | `--perms` |
| **软链接处理** | 保持软链接 | `--links` |
| **列出多余** | 打印目标端多余文件 | `--list-extra` |
| **列出缺失** | 打印源端缺失文件 | `--list-lost` |

---

## 5. 部署与发布

### 5.1 构建

```bash
# 克隆项目
git clone https://github.com/jacksonbean/juicefs-sync-advanced.git
cd juicefs-sync-advanced

# 构建二进制
make build

# 构建产物: juicefs-sync-advanced
```

### 5.2 二进制部署

```bash
# 下载/构建二进制后直接运行
./juicefs-sync-advanced ui --port 9567

# 指定数据库路径
./juicefs-sync-advanced ui \
  --port 9567 \
  --history-db /data/sync_history.db \
  --schedule-db /data/sync_schedule.db
```

### 5.3 CLI 使用

```bash
# 基本同步
./juicefs-sync-advanced sync s3://src-bucket/ s3://dst-bucket/ -p 10

# 增量同步
./juicefs-sync-advanced sync --update s3://src/ s3://dst/

# 带过滤规则
./juicefs-sync-advanced sync \
  --include='*.log' \
  --exclude='*.tmp' \
  s3://src/ s3://dst/

# 定时任务
./juicefs-sync-advanced sync \
  --schedule "0 2 * * *" \
  s3://src/ s3://dst/ -p 20

# 记录到历史数据库
./juicefs-sync-advanced sync s3://src/ s3://dst/ \
  --record-db-type sqlite3 \
  --record-db-dsn ~/.juicefs_sync_history.db \
  --instance-name "daily-backup"
```

### 5.4 Docker 部署

```dockerfile
FROM gcr.io/distroless/static-debian12
COPY juicefs-sync-advanced /usr/local/bin/
EXPOSE 9567
ENTRYPOINT ["/usr/local/bin/juicefs-sync-advanced"]
CMD ["ui", "--port", "9567"]
```

### 5.5 systemd 部署

```ini
[Unit]
Description=JuiceFS Sync Advanced
After=network.target

[Service]
ExecStart=/opt/juicefs-sync-advanced ui --port 9567
Restart=always
User=root

[Install]
WantedBy=multi-user.target
```

---

## 6. 适用场景

### 6.1 适用场景

| 场景 | 推荐理由 |
|------|---------|
| **跨云迁移** | 支持 30+ 存储后端，覆盖主流云厂商 |
| **定时备份** | 集成 Cron 调度 + 任务持久化 |
| **数据同步** | 并发 + 增量同步，效率高 |
| **多活架构** | 多端同步，支持集群模式 |
| **灾备演练** | 支持 dry-run 预演 + 配置复用 |
| **大数据迁移** | HDFS/S3/OSS 等多种存储互相同步 |

### 6.2 不适用场景

| 场景 | 原因 |
|------|------|
| **实时流复制** | 面向对象存储，非流式场景 |
| **目录级同步** | 面向对象 Key，非 POSIX 目录 |
| **双向同步** | 单向同步，不支持冲突处理 |

### 6.3 场景示例

#### 场景 1: 每日增量备份

```bash
# 每天凌晨 2 点增量同步
./juicefs-sync-advanced sync \
  --schedule "0 2 * * *" \
  --update \
  --check-new \
  --instance-name "daily-backup" \
  oss://src-bucket.oss-cn-shanghai.aliyuncs.com/ \
  s3://dst-bucket.s3.us-east-1.amazonaws.com/ \
  -p 20 \
  --record-db-type sqlite3 \
  --record-db-dsn /data/backup_history.db
```

#### 场景 2: 跨云数据迁移

```bash
# 迁移前 dry-run
./juicefs-sync-advanced sync \
  --dry \
  --include='2024/*' \
  --list-extra \
  gcs://src-bucket/ \
  oss://dst-bucket.oss-cn-shanghai.aliyuncs.com/ \
  -p 50

# 正式迁移
./juicefs-sync-advanced sync \
  --include='2024/*' \
  --delete-dst \
  gcs://src-bucket/ \
  oss://dst-bucket.oss-cn-shanghai.aliyuncs.com/ \
  -p 50
```

#### 场景 3: 过滤规则使用

```bash
# 同步特定类型文件，排除临时文件
./juicefs-sync-advanced sync \
  --include='*.{jpg,png,mp4}' \
  --exclude='*.tmp' \
  --exclude='*.bak' \
  --min-size 1M \
  s3://src-bucket/ \
  s3://dst-bucket/
```

#### 场景 4: 高可靠性同步

```bash
# 网络不稳定场景，增加重试次数到 10 次
./juicefs-sync-advanced sync \
  --retry-times 10 \
  --update \
  s3://src-bucket/ \
  oss://dst-bucket.oss-cn-shanghai.aliyuncs.com/ \
  -p 20
```

---

## 7. 存储后端支持

### 7.1 支持的存储类型

| 类别 | 存储 | URI 格式 |
|------|------|---------|
| **云厂商对象存储** | AWS S3 | `s3://bucket/prefix` |
| | 阿里云 OSS | `oss://bucket.endpoint/prefix` |
| | 华为云 OBS | `obs://bucket/prefix` |
| | 腾讯云 COS | `cos://bucket-ap/prefix` |
| | 百度云 BOS | `bos://bucket/prefix` |
| | 七牛云 Kodo | `ks3://bucket/prefix` |
| | UCloud UFile | `ufile://bucket/prefix` |
| | 天翼云 OOS | `oos://bucket/prefix` |
| | 移动云 EOS | `eos://bucket/prefix` |
| | 火山引擎 TOS | `tos://bucket/prefix` |
| | Wasabi | `wasabi://bucket/prefix` |
| | Backblaze B2 | `b2://bucket/prefix` |
| | 青云 QingStor | `qingstor://bucket/prefix` |
| **分布式存储** | HDFS | `hdfs://namenode:port/path` |
| | GlusterFS | `gluster://server/volume` |
| | JuiceFS | `jfs://metaurl/path` |
| | Ceph RADOS | `ceph://pool/prefix` |
| | MinIO | `minio://endpoint/bucket/prefix` |
| **协议存储** | SFTP | `sftp://user:pass@host/path` |
| | WebDAV | `webdav://host/path` |
| | NFS | `nfs://host:/path` |
| | CIFS/SMB | `cifs://server/share/path` |
| **数据库存储** | MySQL | `mysql://user:pass@host/db/table` |
| | PostgreSQL | `postgres://user:pass@host/db/table` |
| | SQLite | `sqlite3:///path/to/db` |
| | Redis | `redis://host/db` |
| | etcd | `etcd://endpoints/prefix` |
| | TiKV | `tikv://pd1,pd2,pd3/prefix` |
| **其他** | 本地文件系统 | `/local/path` 或 `file:///local/path` |
| | 内存存储 | `mem://` |
| | Space (S3兼容) | `space://region/bucket/prefix` |
| | Google Cloud Storage | `gs://bucket/prefix` |
| | Azure Blob | `azure://container/prefix` |
| | IBM COS | `ibmcos://bucket/prefix` |
| | 加密存储 | `encrypt://backend` |

### 7.2 URI 格式说明

```
[NAME://][ACCESS_KEY:SECRET_KEY[:TOKEN]@]BUCKET[.ENDPOINT][/PREFIX]
```

示例:
```bash
# 标准格式
s3://mybucket.s3.us-east-1.amazonaws.com/prefix

# 带认证
oss://AK:SK@mybucket.oss-cn-shanghai.aliyuncs.com/data/

# 带 Token
s3://AK:SK:TOKEN@mybucket.s3.amazonaws.com/
```

---

## 8. 性能调优

### 8.1 并发参数

| 参数 | 默认值 | 说明 |
|------|-------|------|
| `-p, --threads` | 10 | 数据复制并发数 |
| `--list-threads` | 1 | 对象列表并发数 |
| `--list-depth` | 1 | 目录层级并行深度 |

### 8.2 内存与缓冲

| 参数 | 默认值 | 说明 |
|------|-------|------|
| 缓冲区大小 | 32KB | `bufferSize` 常量 |
| 大文件阈值 | 5MB | `defaultPartSize` 常量 |
| 最大块大小 | 10MB | `maxBlock = defaultPartSize * 2` |

### 8.3 调优建议

```bash
# 小文件场景：增加并发
./juicefs-sync-advanced sync s3://src/ s3://dst/ -p 50

# 大文件场景：增加并发 + 调整分片大小
./juicefs-sync-advanced sync s3://src/ s3://dst/ -p 20 --part-size 100M

# 带宽限制场景
./juicefs-sync-advanced sync s3://src/ s3://dst/ -p 10 --bwlimit 100
```

### 8.4 全局限流

```bash
# 配合全局限流服务
./juicefs-sync-advanced sync \
  --traffic-control-url "http://ratelimit:8080/request" \
  s3://src/ s3://dst/
```

---

## 9. 监控与运维

### 9.1 Prometheus 指标

```bash
# 启用指标导出
./juicefs-sync-advanced sync s3://src/ s3://dst/ \
  --metrics 127.0.0.1:9567
```

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `juicefs_sync_copied_objects_total` | Counter | 已复制对象总数 |
| `juicefs_sync_copied_bytes_total` | Counter | 已复制字节总数 |
| `juicefs_sync_checked_objects_total` | Counter | 已校验对象总数 |
| `juicefs_sync_skipped_objects_total` | Counter | 已跳过对象总数 |
| `juicefs_sync_failed_objects_total` | Counter | 失败对象总数 |
| `juicefs_sync_cpu_usage` | Gauge | CPU 使用时间(秒) |
| `juicefs_sync_memory` | Gauge | 内存使用(字节) |
| `juicefs_sync_uptime` | Gauge | 运行时间(秒) |

### 9.2 Consul 注册

```bash
# 注册到 Consul
./juicefs-sync-advanced sync s3://src/ s3://dst/ \
  --consul 127.0.0.1:8500
```

### 9.3 日志级别

```bash
# 调试模式
./juicefs-sync-advanced --debug sync s3://src/ s3://dst/

# 静默模式
./juicefs-sync-advanced --quiet sync s3://src/ s3://dst/

# 指定日志级别
./juicefs-sync-advanced --log-level trace sync s3://src/ s3://dst/
```

### 9.4 Pyroscope 性能分析

```bash
# 启用性能分析
./juicefs-sync-advanced --pyroscope http://pyroscope:4040 sync s3://src/ s3://dst/
```

---

## 10. 常见问题

### Q1: 如何实现断点续传？

大文件(>5MB)使用 MPU (Multipart Upload)，中断后可从断点恢复。任务记录通过 SQLite 持久化，重启后可继续。

### Q2: 如何处理同步失败的对象？

```bash
# Web UI 查看失败记录
# 历史页面可下载失败对象 CSV

# 或 CLI 查看
./juicefs-sync-advanced sync --record-db-type sqlite3 \
  --record-db-dsn history.db \
  s3://src/ s3://dst/
```

### Q3: 如何验证同步完整性？

```bash
# 同步后校验所有文件
./juicefs-sync-advanced sync --check-all s3://src/ s3://dst/

# 仅校验新复制的文件
./juicefs-sync-advanced sync --check-new s3://src/ s3://dst/
```

### Q4: 如何限制同步范围？

```bash
# 按 key 范围
./juicefs-sync-advanced sync --start "2024/" --end "2024/z" s3://src/ s3://dst/

# 按时间
./juicefs-sync-advanced sync --start-time "2024-01-01" --end-time "2024-12-31" s3://src/ s3://dst/

# 按文件大小
./juicefs-sync-advanced sync --min-size 1M --max-size 100G s3://src/ s3://dst/
```

### Q5: 如何处理跨时区问题？

使用 `--start-time` 和 `--end-time` 时注意时区统一，建议使用 UTC 时间或明确指定时区。

### Q6: 集群模式下如何部署？

```bash
# Manager 节点
./juicefs-sync-advanced sync --manager 0.0.0.0:6379 \
  s3://src/ s3://dst/ --instance-name "cluster-sync"

# Worker 节点
./juicefs-sync-advanced sync --worker worker1,worker2 \
  --manager-addr 192.168.1.10:6379 \
  s3://src/ s3://dst/
```

---

## 附录

### A. 版本信息

- **当前版本**: v1.0.0
- **构建时间**: 2026-02-05
- **Go 版本**: 1.23.0+
- **依赖**: 详见 `go.mod`

### B. 相关链接

- GitHub: https://github.com/jacksonbean/juicefs-sync-advanced
- JuiceFS 官方文档: https://juicefs.com/docs/community/

### C. 许可证

Apache License 2.0