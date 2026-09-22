#!/usr/bin/env python3
"""
自动为proto文件添加HTTP注解的脚本
"""

import re
import sys

# 需要添加HTTP注解的服务列表
HTTP_SERVICES = {
    'SystemService', 'PromptService', 'TTSService',
    'PictureForgeService', 'MediaService', 'ChatService', 'AuthService'
}

def add_http_annotation(rpc_line, service_name, method_name):
    """为RPC方法添加HTTP注解"""
    # 检查是否是流式方法
    is_stream = 'stream' in rpc_line

    # 生成URL路径
    service_path = service_name.replace('Service', '').lower()
    method_path = ''.join(['_' + c.lower() if c.isupper() else c for c in method_name]).lstrip('_')
    url_path = f"/v1/{service_path}/{method_path}"

    # 生成HTTP注解（流式和非流式都使用POST）
    annotation = f"""    option (google.api.http) = {{
      post: "{url_path}"
      body: "*"
    }};"""

    return annotation

def process_proto_file(input_file, output_file):
    """处理proto文件，添加HTTP注解"""
    with open(input_file, 'r', encoding='utf-8') as f:
        lines = f.readlines()

    output_lines = []
    current_service = None
    in_service = False
    skip_next_empty = False

    i = 0
    while i < len(lines):
        line = lines[i]

        # 检测服务定义
        service_match = re.match(r'^service\s+(\w+)\s*{', line)
        if service_match:
            current_service = service_match.group(1)
            in_service = current_service in HTTP_SERVICES
            output_lines.append(line)
            i += 1
            continue

        # 检测服务结束
        if in_service and line.strip() == '}':
            in_service = False
            current_service = None

        # 检测RPC方法
        if in_service:
            rpc_match = re.match(r'\s*rpc\s+(\w+)\s*\([^)]+\)\s*returns\s*\([^)]+\)\s*;?\s*$', line)
            if rpc_match:
                method_name = rpc_match.group(1)
                output_lines.append(line.rstrip().rstrip(';') + ' {\n')

                # 添加HTTP注解
                annotation = add_http_annotation(line, current_service, method_name)
                output_lines.append(annotation + '\n')
                output_lines.append('  }\n')

                # 跳过下一个空行
                skip_next_empty = True
                i += 1
                continue

        # 跳过空行（在添加注解后）
        if skip_next_empty and line.strip() == '':
            skip_next_empty = False
            i += 1
            continue

        output_lines.append(line)
        i += 1

    # 写入输出文件
    with open(output_file, 'w', encoding='utf-8') as f:
        f.writelines(output_lines)

    print(f"✅ 已处理 {input_file} -> {output_file}")
    print(f"✅ 为以下服务添加了HTTP注解: {', '.join(sorted(HTTP_SERVICES))}")

if __name__ == '__main__':
    input_file = 'pkg/va_interface/service.proto'
    output_file = 'pkg/va_interface/service.proto'

    process_proto_file(input_file, output_file)
