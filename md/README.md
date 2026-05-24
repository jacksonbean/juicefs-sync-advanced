# JuiceFS Sync Advanced

**增强版对象存储同步工具** — 基于 JuiceFS sync 命令，专注任务持久化、断点续传、智能过滤和 Web UI 调度。

## 特性亮点

| 特性 | 说明 |
|------|------|
| **任务持久化** | SQLite 记录 job 状态，支持中断后断点续传 |
| **增量同步** | --update 基于 mtime 对比，只同步变化的文件 |
| **正则过滤** | --include-regex / --exclude-regex 支持复杂匹配规则 |
| **Benchmark 调优** | --benchmark 自动分析并推荐最佳并发线程数 |
| **定时调度** | --schedule + cron 调度器，支持 SQLite 持久化 |
| **MPU 断点续传** | 大文件分片上传中断后可从断点恢复 |
| **Web UI** | React + shadcn/ui 单页应用，Go embed 单二进制 |
| **实时监控** | Dashboard 3s 轮询，实时吞吐量图表、进度追踪 |
| **配置模板** | 保存常用配置为模板，一键复用 |
| **历史回看** | 可展开查看每次同步完整配置，一键二次迁移 |
| **邮件通知** | 同步完成后发送邮件通知（成功/失败） |
| **存储后端** | 30+ 种对象存储，3 种 URI 格式 |

## 快速开始

```bash
make build
./juicefs-sync-advanced ui --port 9567
```

## Web UI

访问 http://localhost:9567

| 页面 | 说明 |
|------|------|
| **仪表盘** | 实时同步进度、吞吐量图表、运行实例卡片 |
| **新任务** | 创建同步任务，支持从历史一键导入配置 |
| **模板** | 保存/复用常用同步配置 |
| **调度** | 管理 cron 定时任务 |
| **历史** | 趋势图、展开配置、二次迁移、下载报错 CSV |
| **失败** | 查看/重试失败的对象 |

## CLI 同步

```bash
# 基本同步
./juicefs-sync-advanced sync s3://src/ s3://dst/ -p 10

# 增量同步
./juicefs-sync-advanced sync --update s3://src/ s3://dst/

# 同步并记录到历史数据库（UI 中可见）
./juicefs-sync-advanced sync s3://src/ s3://dst/ -p 10 \
  --record-db-type sqlite3 \
  --record-db-dsn ~/.juicefs_sync_history.db \
  --instance-name "我的同步"

# 定时任务
./juicefs-sync-advanced sync --schedule "0 2 * * *" s3://src/ s3://dst/ -p 20
```

## 目录结构

```
juicefs-sync-advanced/
├── main.go
├── webdist.go
├── cmd/              # CLI 命令
├── pkg/
│   ├── api/          # REST API + 调度器
│   ├── sync/         # 核心同步逻辑
│   ├── object/       # 30+ 存储后端
│   └── ...
└── web/              # React 前端
```

## License

Apache License 2.0
