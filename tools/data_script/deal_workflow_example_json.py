import json
import mysql.connector

# 连接到数据库
db = mysql.connector.connect(
    host="dbm.office.domob-inc.cn",
    user="domob",
    password="domob",
    database="rtb"
)

cursor = db.cursor()

# 获取所有需要处理的记录
cursor.execute("SELECT id, example_json FROM va_workflow WHERE example_json IS NOT NULL")
records = cursor.fetchall()

for record_id, example_json in records:
    try:
        # 解析JSON
        data = json.loads(example_json)

        # 处理每个字典中的ori_content字段
        modified = False
        for item in data:
            if 'ori_content' in item and isinstance(item['ori_content'], str):
                # 将ori_content字符串转换为包含该字符串的数组
                item['ori_content'] = [item['ori_content']]
                modified = True

        # 如果有修改，则更新记录
        if modified:
            new_example_json = json.dumps(data, ensure_ascii=False)
            update_sql = "UPDATE va_workflow SET example_json = %s WHERE id = %s"
            cursor.execute(update_sql, (new_example_json, record_id))
            print(f"已更新ID为{record_id}的记录")
    except json.JSONDecodeError as e:
        print(f"解析ID为{record_id}的JSON时出错: {e}")
    except Exception as e:
        print(f"处理ID为{record_id}的记录时出错: {e}")

# 提交更改并关闭连接
db.commit()
cursor.close()
db.close()

print("处理完成!")
