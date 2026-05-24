# Changelog - JuiceFS Sync Advanced (Merged)

## [2.0.0] - 2026-05-17

### 合并说明
合并 `juicefs-sync-advanced-main` (重构版) 和 `juicefs-sync-advanced` (UI版) 两个分支

### 架构改进
- **SyncState 状态封装**: 所有共享状态封装到结构体，消除全局变量竞争
- **Context 支持**: `Sync(ctx, src, dst, config)` 支持取消操作
- **向后兼容**: `getState(config)` 自动回退到全局状态

### 新增功能
- `--list-extra`: 列出目标端有但源端没有的文件
- `--list-lost`: 列出源端有但目标端没有的文件
- `--record-*`: 完整的同步操作记录系统

### UI 界面
- Streamlit 管理界面 (端口 8501)
- 实时进度监控
- 历史记录查看
- 失败文件分析

### 测试
- 所有测试通过 ✅
- 修复测试以适配新 API

## 文件变更
- `pkg/sync/sync.go` - SyncState, Context 支持
- `pkg/sync/config.go` - 新配置字段
- `pkg/sync/cluster.go` - 状态注入
- `pkg/sync/download.go` - 带状态的下载器
- `cmd/sync.go` - 新 CLI 标志
- `ui/` - 新增 Streamlit 界面
