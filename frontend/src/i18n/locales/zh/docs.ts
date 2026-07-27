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
    quickStart: '快速开始',
    parameters: '参数',
    parameter: '参数',
    type: '类型',
    description: '说明',
    rateLimits: '速率限制',
    rateLimitDescription: '速率与并发限制由 API 密钥、分组、用户和上游账号配置共同决定。429 响应可能包含 Retry-After 重试提示。',
    nav: {
      overview: '快速开始',
      models: '模型与路由',
      apiKeys: 'API 密钥',
      usage: '用量与额度',
      settings: 'SDK 配置',
      authentication: '认证与错误',
      chatCompletion: '对话补全',
      imageGeneration: '图像生成',
      videoGeneration: '视频生成',
      modelList: '模型列表'
    },
    articles: {
      overview: {
        title: '平台概览',
        description: '通过统一网关接入 OpenAI、Anthropic、Gemini、Grok 等平台，并继续使用熟悉的 SDK 和请求格式。',
        sections: {
          start: {
            title: '完成首次调用',
            body: '一次完整接入由密钥、模型和请求端点组成。建议按以下顺序完成配置。',
            items: {
              first: {
                title: '创建并绑定密钥',
                description: '在面板创建 API 密钥并选择目标分组。分组决定可用平台、计费方式、模型范围和功能权限。'
              },
              second: {
                title: '读取模型目录',
                description: '携带该密钥请求 GET /v1/models，获取当前分组实际可用的模型 ID。'
              },
              third: {
                title: '发送业务请求',
                description: '选择与客户端兼容的端点，将模型 ID 原样写入请求。流式与非流式响应沿用对应协议。'
              }
            }
          },
          endpoints: {
            title: '选择兼容端点',
            body: '同一个网关地址提供多种兼容协议。密钥所属分组会参与协议选择和上游路由。',
            items: {
              first: {
                title: 'OpenAI 兼容',
                description: 'POST /v1/chat/completions 与 POST /v1/responses 适用于 OpenAI SDK、Codex 及兼容客户端。'
              },
              second: {
                title: 'Anthropic 与 Gemini',
                description: 'POST /v1/messages 提供 Anthropic Messages 兼容入口；Gemini 原生客户端可使用 /v1beta/models。'
              },
              third: {
                title: '发现与观测',
                description: 'GET /v1/models 返回模型目录，GET /v1/usage 返回当前密钥的余额、额度和调用统计。'
              }
            }
          }
        },
        examples: {
          primary: {
            title: '使用 OpenAI SDK 获取模型',
            description: '替换 baseURL 和 API 密钥后即可复用官方 SDK。模型目录是后续请求的可靠来源。'
          }
        },
        callout: {
          title: '密钥决定可用能力',
          body: '网关会根据密钥绑定分组确定平台、模型映射和功能开关。遇到模型不可用或权限错误时，请先核对密钥分组和 GET /v1/models 的返回结果。'
        }
      },
      models: {
        title: '模型与路由',
        description: '模型 ID 同时用于能力选择和上游路由。可用范围由 API 密钥绑定分组实时确定。',
        sections: {
          selection: {
            title: '选择模型',
            body: '模型名称会随平台更新和管理员配置变化。客户端应从当前密钥的模型目录中读取可选值。',
            items: {
              first: {
                title: '以模型列表为准',
                description: '调用 GET /v1/models，并使用 data 数组中的 id 字段。不同密钥可能获得不同目录。'
              },
              second: {
                title: '完整传递模型 ID',
                description: '保留模型 ID 的大小写、前缀和版本后缀，网关会据此执行模型映射和账号筛选。'
              },
              third: {
                title: '匹配请求能力',
                description: '文本、推理、嵌入、图像和视频能力可能属于不同模型与分组，请选择与目标端点匹配的模型。'
              }
            }
          },
          routing: {
            title: '路由过程',
            body: '网关在请求到达后依次解析分组平台、模型映射和可调度账号，并在可用候选中完成选择。',
            items: {
              first: {
                title: '分组确定平台',
                description: 'API 密钥绑定的分组决定请求进入 OpenAI、Anthropic、Gemini、Grok 或其他兼容处理链路。'
              },
              second: {
                title: '映射确定上游模型',
                description: '管理员可为账号配置模型映射。客户端继续使用公开模型 ID，网关负责转换为上游模型名称。'
              },
              third: {
                title: '调度保证连续性',
                description: '网关会结合账号状态、并发和健康度选择上游；可重试故障可在响应开始前切换候选账号。'
              }
            }
          }
        },
        examples: {
          primary: {
            title: '在请求中指定模型',
            description: '将 GET /v1/models 返回的模型 ID 写入 model 字段。stream 可按客户端需求启用。'
          }
        },
        callout: {
          title: '动态目录优先',
          body: '生产应用可短时缓存模型目录，并在密钥、分组或权限变化后刷新。硬编码模型名称容易在平台升级后产生不可用请求。'
        }
      },
      apiKeys: {
        title: 'API 密钥',
        description: 'API 密钥同时承载身份、平台分组、额度和访问边界，适合按应用或环境独立管理。',
        sections: {
          lifecycle: {
            title: '密钥生命周期',
            body: '为开发、测试和生产环境分别创建密钥，可以简化审计、轮换和故障隔离。',
            items: {
              first: {
                title: '创建与分组',
                description: '在面板的 API 密钥页面填写名称并选择分组。分组会确定该密钥的协议平台和计费规则。'
              },
              second: {
                title: '安全部署',
                description: '通过环境变量或密钥管理服务注入服务端进程，并为不同应用使用独立密钥。'
              },
              third: {
                title: '轮换与撤销',
                description: '先部署新密钥，再停用旧密钥。发现泄露时可立即撤销，并检查对应密钥的用量记录。'
              }
            }
          },
          controls: {
            title: '访问控制',
            body: '密钥可配置多层消费和网络限制，网关会在请求进入上游前完成校验。',
            items: {
              first: {
                title: '总额度',
                description: 'quota 以 USD 计价并累计当前密钥的已用金额。额度耗尽后，相关请求会被拒绝。'
              },
              second: {
                title: '周期限额',
                description: '可分别设置 5 小时、1 天和 7 天消费上限，并在 GET /v1/usage 中查看窗口用量和重置时间。'
              },
              third: {
                title: '有效期与 IP 规则',
                description: '通过过期时间、IP 白名单和 IP 黑名单收紧部署范围，适合固定出口和临时项目。'
              }
            }
          }
        },
        examples: {
          primary: {
            title: '使用环境变量',
            description: '将密钥和基础地址放入部署环境。示例变量名可直接用于后续 SDK 配置。'
          }
        },
        callout: {
          title: '按最小权限拆分密钥',
          body: '前端代码、公开仓库、构建日志和客户端错误信息都可能暴露密钥。浏览器应用应通过自己的服务端调用网关。'
        }
      },
      usage: {
        title: '用量与额度',
        description: '通过 GET /v1/usage 查询当前 API 密钥的有效状态、剩余额度、请求统计和模型消费。',
        sections: {
          query: {
            title: '查询方式',
            body: '用量接口直接使用 API 密钥认证，适合客户端状态页、余额监控和自动告警。',
            items: {
              first: {
                title: '认证',
                description: '在 Authorization: Bearer 请求头中传入要查询的 API 密钥，无需用户会话或面板登录态。'
              },
              second: {
                title: '每日明细',
                description: 'days 参数支持 1 至 90 天，timezone 用于确定每日统计边界，例如 Asia/Shanghai。'
              },
              third: {
                title: '模型统计范围',
                description: 'start_date 与 end_date 使用 YYYY-MM-DD 格式，可限定 model_stats 的聚合时间段。'
              }
            }
          },
          response: {
            title: '理解响应',
            body: '响应会根据密钥的计费方式返回额度模式或余额模式，并附带可用的统计维度。',
            items: {
              first: {
                title: 'mode 与 remaining',
                description: 'quota_limited 表示密钥配置了额度或周期限额；unrestricted 会返回订阅或钱包余额信息。'
              },
              second: {
                title: 'usage 汇总',
                description: 'today 与 total 包含请求数、输入输出 Token、缓存 Token 和费用，并提供 RPM、TPM 与平均耗时。'
              },
              third: {
                title: '趋势与模型分布',
                description: 'daily_usage 提供逐日数据，model_stats 提供模型维度汇总，便于成本拆分和容量规划。'
              }
            }
          }
        },
        examples: {
          primary: {
            title: '查询最近 30 天用量',
            description: 'days 控制每日明细范围，timezone 决定日期切分。基础余额和总用量会一并返回。'
          }
        },
        callout: {
          title: '字段按数据可用性返回',
          body: '额度、订阅、daily_usage 和 model_stats 会随密钥配置与统计服务状态出现。集成时请为可选字段设置合理默认值。'
        }
      },
      settings: {
        title: 'SDK 配置',
        description: '设置基础地址、超时、重试和流式处理，让现有 OpenAI 兼容客户端稳定接入网关。',
        sections: {
          connection: {
            title: '连接配置',
            body: '大多数兼容 SDK 只需要替换 API 密钥和 base URL，业务层请求结构可以保持一致。',
            items: {
              first: {
                title: '基础地址',
                description: 'OpenAI 兼容 SDK 的 base URL 使用当前实例地址并以 /v1 结尾，SDK 会继续拼接具体资源路径。'
              },
              second: {
                title: '服务端密钥',
                description: '从服务端环境变量读取 API 密钥，避免将凭据打包到浏览器或移动客户端。'
              },
              third: {
                title: '模型来源',
                description: '应用启动或配置更新时读取 /v1/models，并将返回的 id 作为 model 参数。'
              }
            }
          },
          production: {
            title: '生产环境建议',
            body: '生成式请求的耗时跨度较大。超时、重试和取消策略应结合流式响应状态设计。',
            items: {
              first: {
                title: '设置完整超时',
                description: '分别考虑连接时间和模型生成时间。长上下文、推理与图像任务通常需要更长响应超时。'
              },
              second: {
                title: '限制自动重试',
                description: '对连接失败、429 和可恢复的 5xx 使用指数退避，并设置明确的最大重试次数。'
              },
              third: {
                title: '正确处理流式响应',
                description: '收到首个事件后按协议持续消费数据；用户取消时应同步中止上游请求并释放连接。'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'TypeScript 客户端配置',
            description: '以下配置适用于 openai npm 包。超时和重试值可根据模型类型与业务 SLA 调整。'
          }
        },
        callout: {
          title: '流开始后避免整请求重试',
          body: '流式响应已经输出部分内容时，客户端重放请求可能产生重复结果和重复计费。建议将此类失败交给业务层明确处理。'
        }
      },
      authentication: {
        title: '认证与错误',
        description: '所有网关接口均使用 API 密钥认证。Bearer 请求头具有最广泛的客户端兼容性。',
        sections: {
          headers: {
            title: '认证请求头',
            body: '网关支持多种兼容请求头，服务端集成推荐统一使用 Authorization: Bearer。',
            items: {
              first: {
                title: 'Authorization',
                description: '推荐格式为 Authorization: Bearer YOUR_API_KEY，适用于 /v1 下的主要网关接口。'
              },
              second: {
                title: '兼容请求头',
                description: 'Anthropic 风格客户端可使用 x-api-key；Gemini 客户端还可使用 x-goog-api-key。'
              },
              third: {
                title: '请求体格式',
                description: 'JSON 请求发送 Content-Type: application/json；图像编辑等上传接口根据协议使用 multipart/form-data。'
              }
            }
          },
          errors: {
            title: '常见状态码',
            body: '错误响应会包含类型和说明。客户端应结合 HTTP 状态码决定刷新凭据、调整权限或延迟重试。',
            items: {
              first: {
                title: '401 身份错误',
                description: '密钥缺失、格式错误、已停用或无法识别。检查请求头并确认密钥当前状态。'
              },
              second: {
                title: '403 权限错误',
                description: '密钥尚未分组，或目标功能在分组中未启用。请核对分组、模型和功能开关。'
              },
              third: {
                title: '429 额度或并发限制',
                description: '额度、周期限额、用户并发或图像并发达到上限。读取 Retry-After 后采用退避策略。'
              }
            }
          }
        },
        examples: {
          primary: {
            title: '发送认证请求',
            description: '使用模型列表接口验证密钥和分组配置，是部署后的快速连通性检查。'
          }
        },
        callout: {
          title: '保护凭据和日志',
          body: '日志中应记录请求 ID、状态码和错误类型，并对 Authorization、x-api-key、x-goog-api-key 等字段执行脱敏。'
        }
      },
      chatCompletion: {
        title: '对话补全 API',
        description: '面向所有已支持大语言模型的统一网关接口，针对高吞吐和低延迟场景设计。'
      },
      imageGeneration: {
        title: '图像生成 API',
        description: '通过 OpenAI 兼容接口生成或编辑图像，并沿用 API 密钥的分组、额度和调度规则。',
        sections: {
          generation: {
            title: '快速开始',
            body: '图像生成使用 JSON 请求。模型、尺寸、质量和输出格式的实际可选值由上游模型能力决定。',
            items: {
              first: {
                title: '请求端点',
                description: '向 POST /v1/images/generations 发送请求。model 和 prompt 是业务侧最重要的输入。'
              },
              second: {
                title: '生成参数',
                description: '可按模型支持情况传入 size、quality、n 与 response_format。网关会保持兼容字段并转发。'
              },
              third: {
                title: '读取结果',
                description: '成功响应通常在 data 数组中返回 b64_json 或 url，并可能包含修订后的提示词。'
              }
            }
          },
          editing: {
            title: '编辑与运行限制',
            body: '图像编辑和生成共享认证、计费与调度链路，编辑请求还需要满足上游账号的对应能力。',
            items: {
              first: {
                title: '图像编辑',
                description: 'POST /v1/images/edits 接收源图、提示词和模型。文件上传场景使用 multipart/form-data。'
              },
              second: {
                title: '分组能力',
                description: '当前接口面向启用图像生成的 OpenAI 或 Grok 分组，并要求分组内存在支持目标模型的可调度账号。'
              },
              third: {
                title: '并发与计费',
                description: '图像任务会占用独立并发槽位并记录用量。429 表示当前额度或并发需要等待释放。'
              }
            }
          }
        },
        examples: {
          primary: {
            title: '生成一张图像',
            description: '先从 /v1/models 确认可用图像模型，再根据该模型支持的尺寸和质量参数发送 JSON 请求。'
          },
          edit: {
            title: '上传参考图进行编辑',
            description: '编辑接口使用 multipart/form-data。image 传入本地源图，mask、input_fidelity、background 和 output_format 可按模型能力继续添加。'
          },
          response: {
            title: '解析生成结果',
            description: 'response_format=b64_json 时，解码 data[].b64_json 并保存为图像；response_format=url 时，请及时下载 data[].url 指向的文件。'
          }
        },
        callout: {
          title: '先检查分组功能',
          body: '图像接口需要分组开启图像生成权限。模型不可用、功能关闭或上游账号缺少生成能力时，网关会返回明确的权限或路由错误。'
        }
      },
      videoGeneration: {
        title: '视频生成 API',
        description: '通过异步视频接口调用 HappyHorse，支持文生视频、首帧图生视频、参考生视频和视频编辑。',
        sections: {
          capabilities: {
            title: 'HappyHorse 能力与输入',
            body: 'HappyHorse 1.0 和 1.1 均提供四类独立模型。图片与视频输入使用生成服务可以访问的公网 HTTPS URL。',
            items: {
              first: {
                title: '模型名称',
                description: '使用 happyhorse-1.0 或 happyhorse-1.1，并按能力选择 -t2v、-i2v、-r2v 或 -video-edit 后缀。'
              },
              second: {
                title: '媒体字段',
                description: '首帧图生视频使用 first_frame，参考生视频使用 reference_images，视频编辑使用 video，并可通过 image 添加参考图。'
              },
              third: {
                title: '生成参数',
                description: 'resolution 支持 720P 和 1080P；ratio 默认 16:9；seconds 或 duration 设置输出时长，省略时使用 5 秒。'
              }
            }
          },
          lifecycle: {
            title: '提交、查询与结算',
            body: '视频采用异步任务模式。创建接口立即返回任务 ID，Sub2API 后台轮询上游并将任务状态和结果统一暴露给客户端。',
            items: {
              first: {
                title: '创建任务',
                description: 'POST /v1/videos 创建生成任务，/v1/videos/generations 是兼容路径；视频编辑使用 POST /v1/videos/edits。'
              },
              second: {
                title: '查询状态',
                description: '通过 GET /v1/videos/:id 查询 queued、in_progress、settling、completed 或 failed 状态。completed 响应包含 video_url。'
              },
              third: {
                title: '计费与结果地址',
                description: '提交时预扣 Credits，成功后按实际输出结算，失败时释放预扣额度。expires_at 表示签名结果地址的过期时间。'
              }
            }
          }
        },
        examples: {
          textToVideo: {
            title: '文生视频',
            description: '选择 -t2v 模型并提供 prompt。以下 5 秒 720P 请求适合首次连通性验证。'
          },
          imageToVideo: {
            title: '图生视频（基于首帧）',
            description: '选择 -i2v 模型，通过 first_frame 提供首帧图片 URL。image 可作为兼容字段。'
          },
          referenceToVideo: {
            title: '参考生视频',
            description: '选择 -r2v 模型，通过 reference_images 数组提供一个或多个参考图片 URL。'
          },
          videoEdit: {
            title: '视频编辑',
            description: '选择 -video-edit 模型，通过 video 提供待编辑视频 URL，并可通过 image 提供参考图片。'
          },
          status: {
            title: '查询视频任务',
            description: '将创建响应中的 id 替换到查询路径。客户端可每 5 至 10 秒轮询一次，直到进入 completed 或 failed。'
          }
        },
        callout: {
          title: '使用可公开访问的媒体地址',
          body: '上游会直接下载 first_frame、reference_images、video 和 image 指向的资源。请确保 URL 在任务执行期间有效，并在 expires_at 前保存生成结果。'
        }
      },
      modelList: {
        title: '模型列表 API',
        description: '通过 GET /v1/models 获取当前 API 密钥可以实际选择的模型目录。',
        sections: {
          catalog: {
            title: '返回内容',
            body: '模型列表兼容 OpenAI 的 list 结构，并根据密钥分组的平台和模型配置生成。',
            items: {
              first: {
                title: '按密钥隔离',
                description: '相同网关地址下，不同密钥可能看到不同模型目录。认证密钥始终参与列表计算。'
              },
              second: {
                title: '读取 data[].id',
                description: 'object 固定为 list，data 中每个条目的 id 是请求 model 字段应使用的值。'
              },
              third: {
                title: '平台字段差异',
                description: 'OpenAI 与其他兼容平台可能返回不同的辅助字段。跨平台客户端只需依赖 id。'
              }
            }
          },
          refresh: {
            title: '刷新策略',
            body: '目录会跟随账号映射、分组自定义列表和平台默认模型变化，客户端应保留刷新机制。',
            items: {
              first: {
                title: '短时缓存',
                description: '可在应用内短时缓存结果以减少重复请求，并为用户提供手动刷新入口。'
              },
              second: {
                title: '配置变化后刷新',
                description: '切换 API 密钥、修改分组或收到 model_not_found 后，应立即重新获取目录。'
              },
              third: {
                title: '可用性仍会变化',
                description: '模型目录描述配置能力；瞬时账号容量和上游状态仍可能影响单次请求。'
              }
            }
          }
        },
        examples: {
          primary: {
            title: '读取当前模型目录',
            description: '请求必须携带目标 API 密钥。返回的模型 ID 可直接用于对话、Responses 或对应能力端点。'
          }
        },
        callout: {
          title: '避免维护静态模型表',
          body: '将 /v1/models 作为模型选择器的数据源，可以让客户端自动跟随管理员配置和上游模型更新。'
        }
      }
    },
    parameterRows: {
      model: '必填。要使用的模型 ID，可在模型列表中查看可选值。',
      messages: '必填。由当前对话消息组成的数组。',
      temperature: '可选，默认值为 1。采样温度范围为 0 到 2。',
      maxTokens: '可选。本次补全最多生成的 Token 数量。'
    }
  }
}
