# PC_SDK通讯协议

## 概括
GUI程序与SDK程序使用标准输入/输出(stdin/stdout)通道进行通讯，每条通讯消息都是json格式，并且消息间以换行符'\n'分隔，消息内不允许带有分隔符'\n'，防止SDK读取消息失败。

## 消息类型
以下json格式为了直观使用了换行符隔开，注意传入SDK时不允许在json消息内带'\n'

### 命令
* 初始化
```json
// 请求
{
    "type": "command",
    // 事务ID，接入端自定义，用于对应响应消息
    "txid": 100,
    "label": "init",
    "payload": {
        // 私钥（hex编码）
        "private_key": "ea68cb96eac0fadae73aa71d77b1bcc59d158194e9741377376c3fc177182f1a",
        // 数据库目录
        "data_dir": "C:\\Users\\Administrator\\.aks_sdk\\debug\\1",
        // 加入网络的引导节点
        "bootstrap": [
            "/ip4/120.79.167.233/udp/8715/quic-v1/p2p/16Uiu2HAm9Z4685GEEbo9HXTJa7CnE4unZBNGamYYRN7iuLmFkaM8",
            "/ip4/13.115.253.200/udp/8715/quic-v1/p2p/16Uiu2HAmDSimUfxdm21Bmkbk1XQxJhUaSXvRoFvbDE7CVsBhTjcz",
            "/ip4/13.42.30.173/udp/8715/quic-v1/p2p/16Uiu2HAm5HoYJGWSuazagqNfUjjtK8qQ7gKt87NCYrUKqHENz3NE",
            "/ip4/47.128.241.247/udp/8715/quic-v1/p2p/16Uiu2HAmA5v6BXKQR9aF57fX15tqkTv2GMXCrHSUUYMi1DoofeDf"
        ]
    }
}

// 成功响应
{
    "type": "response",
    "txid": 100,
    // 成功code为1
    "code": 1,
    "payload": {
        // 本节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 本节点EVM地址
        "evm_addr": "0x6b13D7fDc5720D32847b04147331Eb7663E0B0E2"
    }
}

// 失败响应
{
    "type": "response",
    "txid": 100,
    // 失败code为负数
    "code": -1,
    "payload": "fail reason"
}
```
* 查询会话列表
> 注意：**查询会话列表** 的前置条件为 **初始化**
```json
// 请求
{
    "type": "command",
    "txid": 101,
    "label": "peers"
}

// 成功响应
{
    "type": "response",
    "txid": 101,
    "code": 1,
    "payload": [
        {
            // 对方的节点ID
            "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
            // 对方的EVM地址
            "evm_addr": "0x6b13D7fDc5720D32847b04147331Eb7663E0B0E2",
            // 对方的备注
            "remark": "Mary",
            // 是否置顶
            "pin": true,
            // 会话中最后一条消息
            "last_message": {
                // 对方的节点ID
                "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
                // 此条消息的ID
                "message_id": "1",
                // 消息类型，文本为text/plain
                "type": "text/plain",
                // 消息内容，需要base64编码，当为图片类型时，返回的是本地图片路径
                "payload": "aGVsbG8=",
                // 是否已送达
                "reached": true,
                // 是否发起方
                "initiator": true,
                // 创建时间毫秒时间戳
                "created_at": 1734938614411,
                // 用于“阅后即焚”，消息有效时间（毫秒数），为空时永久有效
                "ttl": 5000
            }
        },
        {
            "peer_id": "16Uiu2HAmP9zMUCWkv5FipRvTD7thXdNNsK3gAVNycid1Z9booreb",
            "evm_addr": "0x6b13D7fDc5720D3284ab04147331Eb7663E0Baa3",
            "remark": "Lucy",
            "pin": false,
            "last_message": {
                "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
                "message_id": "1",
                "type": "text/plain",
                "payload": "aGVsbG8=",
                "reached": true,
                "initiator": true,
                "created_at": 1734938614411
            }
        }
    ]
}

// 失败响应
{
    "type": "response",
    "txid": 101,
    "code": -1,
    "payload": "fail reason"
}
```
* 置顶对方用户
> 注意：**置顶对方用户** 的前置条件为 **初始化**
```json
// 请求
{
    "type": "command",
    "txid": 102,
    "label": "pin_peer",
    "payload": {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 是否置顶
        "pin": true
    }
}

// 成功响应
{
    "type": "response",
    "txid": 102,
    "code": 1
}

// 失败响应
{
    "type": "response",
    "txid": 102,
    "code": -1,
    "payload": "fail reason"
}
```
* 备注对方用户
> 注意：**备注对方用户** 的前置条件为 **初始化**
```json
// 请求
{
    "type": "command",
    "txid": 103,
    "label": "set_remark",
    "payload": {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 对方的备注
        "remark": "Amy"
    }
}

// 成功响应
{
    "type": "response",
    "txid": 103,
    "code": 1
}

// 失败响应
{
    "type": "response",
    "txid": 103,
    "code": -1,
    "payload": "fail reason"
}
```
* 清空移除对话框
> 注意：**清空移除对话框** 的前置条件为 **初始化**
```json
// 请求
{
    "type": "command",
    "txid": 104,
    "label": "delete_dialogue",
    "payload": {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg"
    }
}

// 成功响应
{
    "type": "response",
    "txid": 104,
    "code": 1
}

// 失败响应
{
    "type": "response",
    "txid": 104,
    "code": -1,
    "payload": "fail reason"
}
```
* 打开会话
> 注意：**打开会话** 的前置条件为 **初始化**
```json
// 请求
{
    "type": "command",
    "txid": 200,
    "label": "open",
    "payload": {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg"
    }
}

// 成功响应
{
    "type": "response",
    "txid": 200,
    "code": 1
}

// 失败响应
{
    "type": "response",
    "txid": 200,
    "code": -1,
    "payload": "fail reason"
}
```
* 发送消息
> 注意：**发送消息** 的前置条件为 **打开会话**
```json
// 请求
{
    "type": "command",
    "txid": 300,
    "label": "send",
    "payload": {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 消息类型，文本为text/plain
        "type": "text/plain",
        // 消息内容，需要base64编码，当为图片类型时，传入图片路径
        "payload": "aGVsbG8=",
        // 用于“阅后即焚”，消息有效时间（毫秒数），为空时永久有效，仅支持文本类型
        "ttl": 5000
    }
}

// 成功响应
{
    "type": "response",
    "txid": 300,
    "code": 1,
    "payload": {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 此条消息的ID
        "message_id": "1",
        // 消息类型，文本为text/plain
        "type": "text/plain",
        // 消息内容，需要base64编码，当为图片类型时，返回的是本地图片路径
        "payload": "aGVsbG8=",
        // 是否已送达
        "reached": true,
        // 是否发起方
        "initiator": true,
        // 创建时间毫秒时间戳
        "created_at": 1734938614411,
        // 用于“阅后即焚”，消息有效时间（毫秒数），为空时永久有效
        "ttl": 5000
    }
}

// 失败响应
{
    "type": "response",
    "txid": 300,
    "code": -1,
    "payload": "fail reason"
}
```
* 提取历史聊天
> 注意：**提取历史聊天** 的前置条件为 **打开会话**

提取最近的N条历史记录，并以时间正序排序
```json
// 请求
{
    "type": "command",
    "txid": 301,
    "label": "fetch_history",
    "payload": {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 偏移值
        "offset": 0,
        // 提取条数
        "limit": 20
    }
}

// 成功响应
{
    "type": "response",
    "txid": 301,
    "code": 1,
    "payload": [
        {
            // 对方的节点ID
            "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
            // 此条消息的ID
            "message_id": "1",
            // 消息类型，文本为text/plain
            "type": "text/plain",
            // 消息内容，需要base64编码，当为图片类型时，返回的是本地图片路径
            "payload": "aGVsbG8=",
            // 是否已送达
            "reached": true,
            // 是否发起方
            "initiator": true,
            // 创建时间毫秒时间戳
            "created_at": 1734938614411,
            // 用于“阅后即焚”，消息有效时间（毫秒数），为空时永久有效
            "ttl": 5000
        },
        {
            "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
            "message_id": "2",
            "type": "text/plain",
            "payload": "aGVsbG8=",
            "reached": true,
            "initiator": true,
            "created_at": 1734938614413
        }
    ]
}

// 失败响应
{
    "type": "response",
    "txid": 301,
    "code": -1,
    "payload": "fail reason"
}
```
* 删除聊天记录
> 注意：**删除聊天记录** 的前置条件为 **打开会话**
```json
// 请求
{
    "type": "command",
    "txid": 302,
    "label": "delete_message",
    "payload": {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 消息ID数组
        "message_ids": [
            "1",
            "2"
        ]
    }
}

// 成功响应
{
    "type": "response",
    "txid": 302,
    "code": 1
}

// 失败响应
{
    "type": "response",
    "txid": 302,
    "code": -1,
    "payload": "fail reason"
}
```
* 关闭释放

SDK关闭并释放网络的连接，调用后若想重新加入网络，需要重新 **初始化**
```json
// 请求
{
    "type": "command",
    "txid": 400,
    "label": "close"
}

// 成功响应（不会失败）
{
    "type": "response",
    "txid": 400,
    "code": 1
}
```
### 响应码
| 响应码 | 详情 |
| :---: | :---: |
| 1 | 成功 |
| -1 | 失败 |
| -2 | 非法参数 |

### 消息类型
| 消息类型 | 详情 |
| :---: | :---: |
| text/plain | 文本类型 |
| image/any | 图片类型 |

### 异步事件
SDK内产生的事件会异步通知给GUI
* 收到消息
> 注意：**收到消息** 的前置条件为 **初始化**
```json
{
    "type": "event",
    "label": "message_received",
    "payload":  {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 此条消息的ID
        "message_id": "1",
        // 消息类型，文本为text/plain
        "type": "text/plain",
        // 消息内容，需要base64编码，当为图片类型时，返回的是本地图片路径
        "payload": "aGVsbG8=",
        // 是否已送达
        "reached": true,
        // 是否发起方
        "initiator": true,
        // 创建时间毫秒时间戳
        "created_at": 1734938614411,
        // 用于“阅后即焚”，消息有效时间（毫秒数），为空时永久有效
        "ttl": 5000
    }
}
```
* 与对端节点连接状态
> 注意：**与对端节点连接状态** 的前置条件为 **打开会话**
```json
{
    "type": "event",
    "label": "peer_status",
    "payload":  {
        // off（离线）、relay（中继）、direct（直连）共三种状态
        "status": "direct"
    }
}
```
* 消息送达
> 注意：**消息送达** 的前置条件为 **发送消息**
```json
{
    "type": "event",
    "label": "message_arrived",
    "payload":  {
        // 对方的节点ID
        "peer_id": "16Uiu2HAmP9zMUCWkvcFipRvTD7thXdNNsK3RAVNycid1Z9booreg",
        // 送达消息的ID
        "message_id": "1"
    }
}
```