# Database Updates for AI Prompt Versioning and Editor Feedback

Target: MySQL 5.7+/8.0. Assumes migration is applied before deploying this app; app requires prompt_version.

## 1. Add `prompt_version` to `venue_validation_histories`

Purpose: track which prompt version produced each validation entry.

```sql
-- Up
ALTER TABLE venue_validation_histories
  ADD COLUMN prompt_version VARCHAR(32) NULL AFTER ai_output_data,
  ADD INDEX idx_vvh_prompt_version (prompt_version);

-- Down (if needed)
ALTER TABLE venue_validation_histories
  DROP INDEX idx_vvh_prompt_version,
  DROP COLUMN prompt_version;
```

Notes:•Nullable to keep existing rows valid and to allow app to run before migration.•Typical values: system@v1, unified_user@v2, etc.

2. New table: editor_feedbackPurpose: collect human reviewer feedback tied to a venue and the prompt version used.-- Up
   CREATE TABLE IF NOT EXISTS editor_feedback (
   id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
   venue_id BIGINT NOT NULL,
   prompt_version VARCHAR(32) NULL,
   feedback_type ENUM('thumbs_up','thumbs_down') NOT NULL,
   comment TEXT NULL,
   ip VARBINARY(16) NULL,
   created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
   PRIMARY KEY (id),
   KEY idx_editor_feedback_venue_id (venue_id),
   KEY idx_editor_feedback_prompt_version (prompt_version),
   KEY idx_editor_feedback_created_at (created_at)
   ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Down
DROP TABLE IF EXISTS editor_feedback;Notes:•ip uses VARBINARY(16) to support IPv4/IPv6 if you decide to store parsed bytes.•prompt_version is nullable; when unknown, leave NULL.•Add FK if desired: FOREIGN KEY (venue_id) REFERENCES venues(id) (ensure matching types/charset/engine).3. Optional indexes (if your table is large)-- Querying latest histories quickly
CREATE INDEX idx_vvh_processed_at ON venue_validation_histories (processed_at DESC);4. Deployment order1)Apply the venue_validation_histories.prompt_version migration.2)Deploy the application (this app requires prompt_version and will write it).3)Create editor_feedback (UI pages can ship earlier; writes will be no-ops until table exists if you gate them in your app).5. Rollback plan•App requires prompt_version; removing it will break scoring/history writes.6. Data backfill (optional)•You can backfill prompt_version as system@v1 for older rows if you want a uniform filter in analytics:UPDATE venue_validation_histories
SET prompt_version = 'system@v1'
WHERE prompt_version IS NULL;
### 3) `.env.dist` additions for the new configuration

Below is a concise block you can paste into `.env.dist`. Values are examples; adjust per environment.

```bash
# --- AI / OpenAI ---
# Model and generation controls
OPENAI_MODEL=gpt-4o-mini
OPENAI_TEMPERATURE=0.1
OPENAI_MAX_TOKENS=250
OPENAI_REQUEST_TIMEOUT_SECONDS=60

# Batch processing
OPENAI_MAX_BATCH_SIZE=5

# --- Prompts ---
# Directory to look for prompt templates before falling back to embedded ones.
# If empty, only embedded prompts are used.
PROMPT_DIR=./prompts

# If true, only use stable prompt variants (ignore experimental weights).
PROMPT_STABLE_ONLY=false

# Weighted selection across versions, comma-separated name=weight.
# Example: prefer v1 (stable) 70%, v2 (experimental) 30%.
PROMPT_WEIGHTS=unified_user@v1=70,unified_user@v2=30

# --- Config hot reload ---
# Optional path to a .env file that the watcher polls for changes.
CONFIG_FILE=.env
# Poll interval in seconds for config reload and prompt directory rescans.
CONFIG_RELOAD_INTERVAL_SECONDS=2

# --- Logging / Metrics / Profiling (existing keys kept for completeness) ---
LOG_LEVEL=info
LOG_FORMAT=json
ENABLE_FILE_LOGGING=true
LOG_FILE=/var/log/venue-validation/app.log

METRICS_ENABLED=true
METRICS_PATH=/metrics

PROFILING_ENABLED=true
PROFILING_PORT=6060

# --- Database pool (existing) ---
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=15
DB_CONN_MAX_LIFETIME_MINUTES=10
DB_CONN_MAX_IDLE_TIME_MINUTES=5Notes on current wiring:•The code already supports CONFIG_FILE and the polling watcher. If you set it, the watcher reloads env from that file on mtime changes.•The AI-specific knobs (OPENAI_*, prompt weighting) are part of the planned configuration surface. If some aren’t yet referenced in code, keep them here for parity with environments; we’ll wire them in as we finalize the prompt manager enhancements and scorer options.