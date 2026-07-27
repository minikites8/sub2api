export default {
  docsPage: {
    pageTitle: 'API 文档',
    primaryNavigation: '主导航',
    mobileNavigation: '文档导航',
    openNavigation: '打开文档导航',
    closeNavigation: '关闭文档导航',
    search: '搜索文档',
    searchPlaceholder: '搜索指南和 API 参考...',
    searchResults: '搜索结果',
    noResults: '没有找到匹配的文档。',
    closeSearch: '关闭搜索',
    navigation: '接入指南',
    apiReference: 'API 参考',
    docs: '文档',
    pricing: '价格',
    models: '模型广场',
    dashboard: '面板',
    login: '登录',
    contactSupport: '联系客服',
    systemStatus: '系统状态',
    footerDescription: '高性能 AI 网关',
    copy: '复制',
    copied: '已复制',
    nav: {
      overview: '快速开始',
      models: '选择模型',
      apiKeys: 'API 密钥',
      usage: '用量与额度',
      settings: '使用 SDK',
      authentication: '密钥认证',
      chatCompletion: '对话生成',
      imageGeneration: '图像生成',
      videoGeneration: '视频生成',
      modelList: '模型列表'
    },
    articles: {
      overview: {
        title: '快速开始',
        description: '准备好 API 密钥和模型名称后，即可发送第一次对话请求。',
        steps: {
          apiKey: {
            title: '1. 获取 API 密钥',
            description: '登录面板，在 API 密钥页面创建一个密钥。创建后请立即复制并妥善保存。',
            action: '前往 API 密钥'
          },
          model: {
            title: '2. 选择模型',
            description: '打开模型广场，复制一个可用的模型名称。',
            action: '查看模型'
          },
          request: {
            title: '3. 发送请求',
            description: '替换示例中的密钥和模型名称，然后运行命令。收到模型回复即表示接入成功。'
          }
        }
      },
      models: {
        title: '选择模型',
        description: '从模型广场找到可用模型，并把模型名称填入请求。',
        steps: {
          browse: {
            title: '1. 打开模型广场',
            description: '模型广场会列出当前可用的模型及其主要能力。',
            action: '查看模型'
          },
          copy: {
            title: '2. 复制模型名称',
            description: '选择符合需求的模型，复制它的完整名称。'
          },
          use: {
            title: '3. 填入请求',
            description: '将复制的名称作为 model 的值。'
          }
        }
      },
      apiKeys: {
        title: 'API 密钥',
        description: 'API 密钥用于识别请求，请勿公开或提交到代码仓库。',
        steps: {
          create: {
            title: '1. 创建密钥',
            description: '在面板的 API 密钥页面创建一个新密钥。',
            action: '前往 API 密钥'
          },
          save: {
            title: '2. 保存密钥',
            description: '密钥通常只完整显示一次，请立即保存到安全的位置。'
          },
          use: {
            title: '3. 用于请求',
            description: '将密钥放入 Authorization 请求头。'
          }
        }
      },
      usage: {
        title: '用量与额度',
        description: '在面板查看请求数量、消费记录和剩余额度。',
        steps: {
          open: {
            title: '1. 打开用量页面',
            description: '用量页面汇总当前账号的调用记录。',
            action: '查看用量'
          },
          range: {
            title: '2. 选择时间范围',
            description: '按天、周或自定义时间查看使用趋势。'
          },
          quota: {
            title: '3. 查看剩余额度',
            description: '额度不足时，请先充值或联系管理员。'
          }
        }
      },
      settings: {
        title: '使用 SDK',
        description: '兼容 OpenAI SDK，只需设置 API 密钥和服务地址。',
        steps: {
          install: {
            title: '1. 安装 SDK',
            description: '在项目中安装 openai 软件包。'
          },
          request: {
            title: '2. 配置并调用',
            description: '替换示例中的密钥和模型名称，即可发送请求。'
          }
        }
      },
      authentication: {
        title: '密钥认证',
        description: '每个 API 请求都需要携带有效的 API 密钥。',
        steps: {
          header: {
            title: '1. 添加认证请求头',
            description: '使用 Authorization: Bearer YOUR_API_KEY。'
          },
          unauthorized: {
            title: '2. 认证失败时',
            description: '收到 401 时，重新复制密钥并确认密钥仍处于启用状态。'
          },
          limited: {
            title: '3. 请求过于频繁时',
            description: '收到 429 时，稍等片刻后再试。'
          }
        }
      },
      chatCompletion: {
        title: '对话生成',
        description: '发送一组对话消息并获取模型回复。',
        steps: {
          model: {
            title: '1. 选择模型',
            description: '从模型广场复制一个支持对话的模型名称。',
            action: '查看模型'
          },
          request: {
            title: '2. 发送消息',
            description: '填写 API 密钥、模型名称和用户消息，然后运行请求。'
          },
          response: {
            title: '3. 读取回复',
            description: '返回结果中的 assistant 内容就是模型回复。'
          }
        }
      },
      imageGeneration: {
        title: '图像生成',
        description: '选择图像模型，输入提示词并获取生成结果。',
        steps: {
          model: {
            title: '1. 选择图像模型',
            description: '从模型广场选择一个支持图像生成的模型。',
            action: '查看模型'
          },
          request: {
            title: '2. 发送提示词',
            description: '替换密钥、模型名称和提示词，然后运行请求。'
          },
          result: {
            title: '3. 保存图片',
            description: '返回内容可能是图片链接或 Base64 数据，请及时保存。'
          }
        }
      },
      videoGeneration: {
        title: '视频生成',
        description: '提交提示词创建视频任务，再查询生成结果。',
        steps: {
          model: {
            title: '1. 选择视频模型',
            description: '从模型广场选择一个支持视频生成的模型。',
            action: '查看模型'
          },
          request: {
            title: '2. 创建任务',
            description: '替换密钥、模型名称和提示词，然后提交请求。'
          },
          result: {
            title: '3. 获取结果',
            description: '使用返回的任务 ID 查询状态；完成后保存视频链接。'
          }
        }
      },
      modelList: {
        title: '模型列表',
        description: '通过接口获取当前 API 密钥可用的模型名称。',
        steps: {
          request: {
            title: '1. 请求模型列表',
            description: '携带 API 密钥调用 /v1/models。'
          },
          read: {
            title: '2. 复制模型 ID',
            description: '从返回的 data 列表中复制需要的 id。'
          },
          use: {
            title: '3. 用于请求',
            description: '将复制的 id 填入其他请求的 model 字段。'
          }
        }
      }
    }
  }
}
