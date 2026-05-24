export interface InstanceInfo { pid: number; port: number; metrics: string; src: string; dst: string; name: string; start_time: string }
export interface RunSummary { run_id: string; src: string; dst: string; dry: boolean; total_scanned: number; total_bytes: number; copied: number; copied_bytes: number; skipped: number; extra: number; deleted: number; failed: number; started_at: string; finished_at: string; elapsed_ms: number; progress_pct: number; config_json?: string }
export interface RunError { source_id: string; target_id: string; size: number; error_message: string; attempted_at: string }
export interface RunDetail { run: RunSummary; errors: RunError[] }
export interface FailedObject { source_id: string; target_id: string; size: number; status: string; error_message: string; attempted_at: string; retry_count: number }
export interface Template { id: number; name: string; description: string; src_template: string; dst_template: string; threads: number; options: string; created_at: string; updated_at: string }
export interface ScheduleTask { id: number; name: string; src: string; dst: string; cron_expr: string; enabled: boolean; threads: number; options: string; last_run: string; next_run: string; created_at: string }
export interface ScheduleHistory { id: number; task_id: number; started_at: string; finished_at: string; status: string; output: string; error: string; objects_copied: number }
export interface AggregatedMetrics { total: number; running: number; scanned: number; copied: number; failed: number }
