#!/usr/bin/env python3
# sync.py
# 依赖：pip install sqlalchemy pymysql apscheduler tenacity

import logging
from datetime import datetime
from sqlalchemy import create_engine, MetaData, Table, select, text
from sqlalchemy.dialects.mysql import insert as mysql_insert
from apscheduler.schedulers.blocking import BlockingScheduler
from tenacity import retry, wait_exponential, stop_after_attempt

# ========= 1. 配置区（短期脚本，直接在代码里改） =========
# 源库 A 和 目标库 B 的 DSN（MySQL + PyMySQL）
# DSN_A = "mysql+pymysql://visionai:bxB66A5APS0j@dm-dsp-visionai.mysql.in.domob-inc.com:3306/visionai"
# DSN_B = "mysql+pymysql://visionai:bxB66A5APS0j@tc-sg-dm-dsp-visionai.mysql.domob-inc.com:3307/visionai_sandbox"


DSN_A = "mysql+pymysql://domob:domob@dbm.office.domob-inc.cn:3306/rtb"
DSN_B = "mysql+pymysql://domob:domob@dbm.office.domob-inc.cn:3306/rtb"

# 需要同步的表列表tc-sg-dm-dsp-visionai.mysql.domob-inc.com p
# - source_name: 源表名
# - target_name: 目标表名（可选，默认使用源表名）
# - unique_keys: 在 B 库必须为此字段建唯一索引，用于 ON DUPLICATE KEY；不需要去重的表可省略此字段
# - inc_field: 增量字段，支持 DATETIME 或可替代自增主键
TABLES = [
    {
        "source_name": "pay_user_subscription",
        "target_name": "pay_user_subscription_ol",  # 可以省略，将使用 source_name
        "unique_keys": ["subscription_id"],
        "inc_field": "created_at",
        "onlyInsert": True,  # 新增字段，冲突时打印数据但不更新
    },
    {
        "source_name": "pay_apple_notify",
        "inc_field": "created_at",   # 根据插入时间做增量
        # "target_name": "pay_apple_notify_cp",  # 可以省略，将使用 source_name
    },
    {
        "source_name": "pay_alipay_notify",
        "inc_field": "created_at",
    },
    {
        "source_name": "pay_apple_renewal",
        "inc_field": "created_at",
    },
    {
        "source_name": "pay_apple_transaction",
        "inc_field": "created_at",
    },
]

# ========= 2. 初始化 DB 引擎和元数据 =========
engine_a = create_engine(DSN_A, pool_recycle=3600)
engine_b = create_engine(DSN_B, pool_recycle=3600)
metadata = MetaData()

# ========= 3. sync_meta 表，用于记录每张表的最后增量游标 =========
def ensure_sync_meta():
    """在 B 库建表用于存游标（只用主键 id）"""
    ddl = """
    CREATE TABLE IF NOT EXISTS pay_sync_meta (
      table_name VARCHAR(128) PRIMARY KEY,
      last_id BIGINT NOT NULL
    );
    """
    with engine_b.begin() as conn:
        conn.execute(text(ddl))

def get_last_inc(table_name: str):
    """读取上次同步的游标值，首次返回 0"""
    with engine_b.begin() as conn:
        row = conn.execute(
            text("SELECT last_id FROM pay_sync_meta WHERE table_name = :t"),
            {"t": table_name}
        ).fetchone()
        if row:
            return row[0]
        # 首次初始化
        zero_id = 0
        conn.execute(
            text("INSERT IGNORE INTO pay_sync_meta(table_name, last_id) VALUES(:t, :i)"),
            {"t": table_name, "i": zero_id}
        )
        return zero_id

# ========= 4. 原子写入单条记录并更新游标 =========
@retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=1, max=10))
def process_record(table: Table, unique_keys: list, record: dict, table_name: str, onlyInsert: bool = False):
    """
    在同一个事务内完成：
    1) 插入或 upsert 一条 record 到目标表
    2) 更新 pay_sync_meta 的 last_id
    - 如果 onlyInsert=True，冲突时打印数据但不更新
    """
    with engine_b.begin() as tx:
        payload = {k: v for k, v in record.items() if k != "id"}
        
        if unique_keys and not onlyInsert:
            # 默认行为：冲突时更新
            ins = mysql_insert(table).values(**payload)
            update_cols = {k: ins.inserted[k] for k in payload.keys() if k not in unique_keys}
            stmt = ins.on_duplicate_key_update(**update_cols)
        elif unique_keys and onlyInsert:
            # 冲突时打印数据但不更新
            try:
                stmt = table.insert().values(**payload)
                tx.execute(stmt)
            except Exception as e:
                logging.warning(f"冲突数据（表 {table_name}，ID={record['id']}）：{record}")
                # 不更新记录，仅更新游标
        else:
            # 无唯一键，直接插入
            stmt = table.insert().values(**payload)

        # 更新游标
        pk = record["id"]
        tx.execute(
            text("UPDATE pay_sync_meta SET last_id = :i WHERE table_name = :t"),
            {"i": pk, "t": table_name}
        )

# ========= 5. 增量同步主逻辑 =========
def do_sync():
    """对所有配置的表，按主键 id 增量拉数据并写入 B 库"""
    ensure_sync_meta()  # 确保 meta 表已创建

    for cfg in TABLES:
        source_name = cfg["source_name"]
        target_name = cfg.get("target_name", source_name)
        pk_field = "id"  # 假设主键字段为 id，如有不同可扩展
        unique_keys = cfg.get("unique_keys", [])

        metadata.clear()
        source_tbl = Table(source_name, metadata, autoload_with=engine_a)
        target_tbl = Table(target_name, metadata, autoload_with=engine_b)

        # 读取上次同步游标
        last_id = get_last_inc(source_name)

        # 拉取本次增量，限制单批次大小为1000条
        stmt = (
            select(source_tbl)
            .where(source_tbl.c[pk_field] > last_id)
            .order_by(source_tbl.c[pk_field].asc())
            .limit(1000)  # 控制单批大小
        )
        with engine_a.connect() as conn_a:
            rows = conn_a.execute(stmt).fetchall()

        if not rows:
            logging.info(f"{source_name}: 无新增数据，游标 {last_id} 保持不变")
            continue

        # 对每条记录，单条事务内完成写入+游标更新
        for row in rows:
            rec = dict(row._mapping)
            try:
                process_record(target_tbl, unique_keys, rec, source_name, cfg.get("onlyInsert", False))
                logging.info(f"{source_name}: 处理并提交 id={rec['id']}")
            except Exception as e:
                logging.error(f"{source_name}: 处理 id={rec['id']} 失败，终止本次 sync，等待下次重试。原因: {e}")
                # 直接退出，不再处理后续记录
                raise

        logging.info(f"{source_name}: 批次共 {len(rows)} 条处理完毕，最后游标 = {rows[-1]._mapping[pk_field]}")

# ========= 6. 定时任务入口 =========
def main():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(message)s",
    )
    # 先跑一次
    do_sync()

    # 每 5 秒执行一次
    sched = BlockingScheduler(timezone="UTC")
    sched.add_job(do_sync, "interval", seconds=5)
    sched.start()

if __name__ == "__main__":
    main()