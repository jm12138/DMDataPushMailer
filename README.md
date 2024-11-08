# DMDataPushMailer
## 简介
* 一个用于达梦数据库数据的定时数据邮件推送器，可定时定向导出数据库数据并通过邮件推送至指定邮箱。

## 使用指南
* 下载/构建

    * [下载预编译版本](https://github.com/jm12138/DMDataPushMailer/releases)

    * 通过源码构建

        ```bash
        $ goreleaser build --snapshot --clean
        ```

* 运行命令：

    ```bash
    $ DMDataPushMailer -config config.json
    ```

* 配置文件介绍：

    ```json
    {
        // 邮箱配置
        "email": {
            "host": "smtp.qq.com",                          // SMTP 服务器地址
            "port": 465,                                    // SMTP 服务器端口
            "username": "",                                 // 用户名
            "password": ""                                  // 密码
        },
        // 数据库配置
        "db": {
            "host": "",                                     // 数据库服务器地址
            "port": 1521,                                   // 数据库服务器端口
            "username": "",                                 // 用户名
            "password": ""                                  // 密码
        },
        // 邮件配置
        "post": [
            {
                "from": "xxxx@qq.com",                      // 发件人
                "to": [
                    "xxxx@qq.com"                           // 收件人列表，支持多个收件邮箱
                ],
                "subject": "SUBJECT",                       // 邮件标题
                "body": "CONTENT",                          // 邮件正文
                "attachment": [                             // 邮件附件列表，支持多个表格附件
                    {
                        "table": "TEST_01",                 // 数据库表或视图
                        "excel": "01.xlsx"                  // 附件名称
                    },
                    {
                        "table": "TEST_02",
                        "excel": "02.xlsx"
                    }
                ]
            }
        ],
              //  ┌────────────── 分钟 (0 - 59)
              //  │  ┌───────────── 小时 (0 - 23)
              //  │  │ ┌───────────── 每月几号 (1 - 31)
              //  │  │ │ ┌───────────── 月份 (1 - 12)
              //  │  │ │ │ ┌───────────── 星期几 (0 - 6) (周日为0)
              //  │  │ │ │ │
              //  *  * * * *
        "time": "00 08 * * *"                               // 定时表达式
        // 表达式	描述	等式
        // @yearly (or @annually)	每年1月1日 00:00:00 执行一次	0 0 0 1 1 *
        // @monthly	每个月第一天的 00:00:00 执行一次	0 0 0 1 * *
        // @weekly	每周周六的 00:00:00 执行一次	0 0 0 * * 0
        // @daily (or @midnight)	每天 00:00:00 执行一次	0 0 0 * * *
        // @hourly	每小时执行一次	0 0 * * * *
        // @every time	指定时间间隔执行一次，如 @every 5s，每隔5秒执行一次。	0/5 * * * * *             
    }
    ```