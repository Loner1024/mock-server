# LLM Mock Server

> 模拟所有 LLM 供应商 API 的 mock server，专为 Higress e2e 测试设计。

## 使用方法

启动服务器：

```bash
./llm-mock-server --port 3000
```


## 支持的供应商

目前已支持以下 LLM 提供商：

- 360 智脑
- Cloudflare
- DeepSeek
- Dify
- Gemini
- GitHub
- Groq
- MiniMax
- OpenAI
- Together AI
- 百川智能
- 豆包
- 零一万物
- 文心一言
- 智谱 AI
- 阶跃星辰

## 决策端点

`llm-mock-server` 同时模拟两类决策协议，返回值固定，便于 e2e 断言：

- `POST /v1/systemone`：TypeSafe System One 请求，`answers` 以问题名作为 key 返回。
- `POST /v1/decisions`：OpenAI Decisions 请求，`answers` 按问题顺序返回数组。

支持的题型为 `noul`/`predicate`、`choice`、`score` 和 `box`。请求体不合法或题型未知时返回 400。
