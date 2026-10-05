-- SPC 应用 schema（PostgreSQL 16）
-- 监控对象、测量值、冻结基准期、判异告警。

CREATE TABLE IF NOT EXISTS targets (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name           TEXT NOT NULL,
    machine        TEXT NOT NULL DEFAULT '',
    dimension      TEXT NOT NULL DEFAULT '',
    subgroup_size  INTEGER NOT NULL CHECK (subgroup_size BETWEEN 2 AND 10),
    usl            DOUBLE PRECISION,
    lsl            DOUBLE PRECISION,
    rules          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT targets_spec_check CHECK (
        (usl IS NOT NULL OR lsl IS NOT NULL)
        AND (usl IS NULL OR lsl IS NULL OR usl > lsl)
    )
);

CREATE TABLE IF NOT EXISTS measurements (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id  BIGINT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    value      DOUBLE PRECISION NOT NULL,
    seq        BIGINT NOT NULL,
    operator   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 切组只认 seq：唯一约束保证多检验员并发写入也不丢不重不乱序。
CREATE UNIQUE INDEX IF NOT EXISTS measurements_target_seq_uidx
    ON measurements(target_id, seq);
CREATE INDEX IF NOT EXISTS measurements_target_idx ON measurements(target_id, seq);

CREATE TABLE IF NOT EXISTS baselines (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id        BIGINT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    version          INTEGER NOT NULL,
    -- start_group/end_group：该限「判定与显示」的生效子组区间（闭区间，end NULL=至今）。
    -- 首版 start=1；重新基准时新限 start=冻结时总组数+1，旧限 end=冻结时总组数。
    start_group      INTEGER NOT NULL,
    end_group        INTEGER,
    -- basis_start/basis_end：计算该限所用基准期子组范围（仅用于限值/能力溯源）。
    basis_start      INTEGER NOT NULL DEFAULT 1,
    basis_end        INTEGER NOT NULL DEFAULT 1,
    xbar_cl          DOUBLE PRECISION NOT NULL,
    xbar_u           DOUBLE PRECISION NOT NULL,
    xbar_l           DOUBLE PRECISION NOT NULL,
    rbar_cl          DOUBLE PRECISION NOT NULL,
    rbar_u           DOUBLE PRECISION NOT NULL,
    rbar_l           DOUBLE PRECISION NOT NULL,
    sigma            DOUBLE PRECISION NOT NULL,
    cp               DOUBLE PRECISION,
    cpk              DOUBLE PRECISION,
    subgroup_count   INTEGER NOT NULL,
    active           BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS baselines_target_version_uidx
    ON baselines(target_id, version);
-- 每个档案至多一条 active 基准（部分唯一索引）。
CREATE UNIQUE INDEX IF NOT EXISTS baselines_one_active_uidx
    ON baselines(target_id) WHERE active;

CREATE TABLE IF NOT EXISTS alarms (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    baseline_id        BIGINT NOT NULL REFERENCES baselines(id) ON DELETE CASCADE,
    target_id          BIGINT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    version            INTEGER NOT NULL,
    rule               TEXT NOT NULL,
    rule_name          TEXT NOT NULL,
    first_index        INTEGER NOT NULL,
    idx                INTEGER NOT NULL,
    points             JSONB NOT NULL,
    message            TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 同一条告警（同一基准、同规则、同触发窗口）幂等，重放结果稳定。
CREATE UNIQUE INDEX IF NOT EXISTS alarms_dedup_uidx
    ON alarms(baseline_id, rule, first_index, idx);
CREATE INDEX IF NOT EXISTS alarms_target_idx ON alarms(target_id, version, idx);
