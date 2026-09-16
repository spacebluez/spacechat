#!/usr/bin/env python3

import argparse
import http.client
import json
import os
import socket
import sys


class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, socket_path):
        super().__init__("localhost", timeout=30)
        self.socket_path = socket_path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.socket_path)


def clear_history(socket_path):
    connection = UnixHTTPConnection(socket_path)
    try:
        connection.request("POST", "/clear-history", body=b"")
        response = connection.getresponse()
        body = response.read()
        if response.status != 200:
            raise RuntimeError("服务端返回 HTTP {}，请检查 xchat 服务日志".format(response.status))
        result = json.loads(body.decode("utf-8"))
        deleted = result.get("deleted") if isinstance(result, dict) else None
        if type(deleted) is not int or deleted < 0:
            raise RuntimeError("服务端未返回有效的清理结果")
        return deleted
    finally:
        connection.close()


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="清空 XChat 全部聊天记录，并同步刷新在线客户端。请以 root 或 xchat 用户运行。"
    )
    parser.add_argument(
        "--socket",
        default=os.environ.get("XCHAT_ADMIN_SOCKET", "/run/xchat/admin.sock"),
        help="管理 Unix socket 路径（默认 /run/xchat/admin.sock）",
    )
    parser.add_argument("--yes", action="store_true", help="跳过交互确认，立即执行清空")
    arguments = parser.parse_args(argv)
    if not arguments.yes:
        try:
            confirmation = input("将清空全部聊天记录且不可撤销。输入 CLEAR 确认：")
        except (EOFError, KeyboardInterrupt):
            print("\n已取消，未发送清理请求。")
            return 2
        if confirmation.strip() != "CLEAR":
            print("已取消，未发送清理请求。")
            return 2
    try:
        deleted = clear_history(arguments.socket)
    except (OSError, http.client.HTTPException, ValueError, RuntimeError) as error:
        print("清理失败或结果未确认：{}。未自动重试。".format(error), file=sys.stderr)
        return 1
    print("清理成功，已删除 {} 条聊天记录。".format(deleted))
    return 0


if __name__ == "__main__":
    sys.exit(main())
