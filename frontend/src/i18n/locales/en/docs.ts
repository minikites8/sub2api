export default {
  docsPage: {
    pageTitle: 'API Documentation',
    primaryNavigation: 'Primary navigation',
    mobileNavigation: 'Documentation navigation',
    openNavigation: 'Open documentation navigation',
    closeNavigation: 'Close documentation navigation',
    search: 'Search documentation',
    searchPlaceholder: 'Search guides and API reference...',
    searchResults: 'Search results',
    noResults: 'No matching documentation found.',
    closeSearch: 'Close search',
    navigation: 'Getting started',
    apiReference: 'API reference',
    docs: 'Docs',
    pricing: 'Pricing',
    models: 'Models',
    dashboard: 'Dashboard',
    login: 'Sign in',
    contactSupport: 'Contact support',
    systemStatus: 'System status',
    footerDescription: 'High-performance AI gateway',
    copy: 'Copy',
    copied: 'Copied',
    quickStart: 'Quick start',
    parameters: 'Parameters',
    parameter: 'Parameter',
    type: 'Type',
    description: 'Description',
    rateLimits: 'Rate limits',
    rateLimitDescription: 'Rate and concurrency limits follow the API key, group, user, and upstream account configuration. A 429 response may include Retry-After guidance.',
    nav: {
      overview: 'Quick start',
      models: 'Models and routing',
      apiKeys: 'API keys',
      usage: 'Usage and quota',
      settings: 'SDK configuration',
      authentication: 'Authentication and errors',
      chatCompletion: 'Chat completions',
      imageGeneration: 'Image generation',
      videoGeneration: 'Video generation',
      modelList: 'Model list'
    },
    articles: {
      overview: {
        title: 'Platform overview',
        description: 'Connect to OpenAI, Anthropic, Gemini, Grok, and other providers through one gateway while keeping familiar SDKs and request formats.',
        sections: {
          start: {
            title: 'Complete your first request',
            body: 'A complete integration combines an API key, a model, and a compatible endpoint. Follow this order for a reliable first request.',
            items: {
              first: {
                title: 'Create and assign a key',
                description: 'Create an API key in the dashboard and assign a group. The group controls the platform, billing rules, model catalog, and feature access.'
              },
              second: {
                title: 'Read the model catalog',
                description: 'Call GET /v1/models with that key to retrieve the model IDs currently available to its group.'
              },
              third: {
                title: 'Send the request',
                description: 'Choose the endpoint supported by your client and pass the returned model ID unchanged. Streaming follows the selected protocol.'
              }
            }
          },
          endpoints: {
            title: 'Choose a compatible endpoint',
            body: 'The gateway exposes several protocol-compatible endpoints. The API key group participates in protocol selection and upstream routing.',
            items: {
              first: {
                title: 'OpenAI compatible',
                description: 'POST /v1/chat/completions and POST /v1/responses serve OpenAI SDKs, Codex, and compatible clients.'
              },
              second: {
                title: 'Anthropic and Gemini',
                description: 'POST /v1/messages provides an Anthropic Messages interface. Gemini-native clients can use the /v1beta/models routes.'
              },
              third: {
                title: 'Discovery and observability',
                description: 'GET /v1/models returns the model catalog. GET /v1/usage returns balance, quota, and request statistics for the current key.'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'List models with the OpenAI SDK',
            description: 'Set the gateway base URL and API key to reuse the official SDK. The model catalog provides the source of truth for later requests.'
          }
        },
        callout: {
          title: 'The key defines available capabilities',
          body: 'The gateway uses the key group to resolve the platform, model mappings, and feature switches. For routing or permission errors, check the key group and the GET /v1/models response.'
        }
      },
      models: {
        title: 'Models and routing',
        description: 'A model ID selects a capability and participates in upstream routing. Each API key receives a live catalog based on its assigned group.',
        sections: {
          selection: {
            title: 'Select a model',
            body: 'Model names evolve with provider releases and administrator configuration. Read the current catalog for each key.',
            items: {
              first: {
                title: 'Use the model list',
                description: 'Call GET /v1/models and use the id field from the data array. Separate keys can receive separate catalogs.'
              },
              second: {
                title: 'Preserve the complete model ID',
                description: 'Keep the casing, prefix, and version suffix. The gateway uses the full value for model mapping and account selection.'
              },
              third: {
                title: 'Match the endpoint capability',
                description: 'Text, reasoning, embeddings, image, and video workloads can use separate models and groups. Choose a model exposed for the target endpoint.'
              }
            }
          },
          routing: {
            title: 'Routing flow',
            body: 'For each request, the gateway resolves the group platform, model mapping, and schedulable accounts, then selects an available candidate.',
            items: {
              first: {
                title: 'The group selects the platform',
                description: 'The key group sends requests through the configured OpenAI, Anthropic, Gemini, Grok, or compatible processing path.'
              },
              second: {
                title: 'Mappings select the upstream model',
                description: 'Administrators can configure per-account model mappings. Clients keep the public model ID while the gateway translates it upstream.'
              },
              third: {
                title: 'Scheduling maintains continuity',
                description: 'The scheduler evaluates account health, concurrency, and capacity. Recoverable failures can move to another candidate before output begins.'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'Specify a model in the request',
            description: 'Place an ID returned by GET /v1/models in the model field. Enable stream when the client supports incremental output.'
          }
        },
        callout: {
          title: 'Prefer the live catalog',
          body: 'Production applications can cache the catalog briefly and refresh it after key, group, or permission changes. This keeps model selection aligned with provider updates.'
        }
      },
      apiKeys: {
        title: 'API keys',
        description: 'An API key carries identity, platform group, quota, and access boundaries. Separate keys work well for each application and environment.',
        sections: {
          lifecycle: {
            title: 'Key lifecycle',
            body: 'Dedicated development, staging, and production keys simplify auditing, rotation, and incident isolation.',
            items: {
              first: {
                title: 'Create and assign',
                description: 'Create a named key in the dashboard and select a group. The group defines its protocol platform and billing rules.'
              },
              second: {
                title: 'Deploy securely',
                description: 'Inject the key into server processes through environment variables or a secret manager. Give each application its own key.'
              },
              third: {
                title: 'Rotate and revoke',
                description: 'Deploy a new key before deactivating the previous one. When exposure occurs, revoke the key and review its usage records.'
              }
            }
          },
          controls: {
            title: 'Access controls',
            body: 'Keys support layered spending and network limits. The gateway evaluates them before dispatching the request upstream.',
            items: {
              first: {
                title: 'Total quota',
                description: 'Quota is priced in USD and tracks spending for the key. Requests stop when the configured quota is exhausted.'
              },
              second: {
                title: 'Rolling limits',
                description: 'Configure independent 5-hour, 1-day, and 7-day spending limits. GET /v1/usage exposes window usage and reset times.'
              },
              third: {
                title: 'Expiration and IP rules',
                description: 'Expiration dates, IP allowlists, and IP blocklists scope a key to approved deployments, fixed egress addresses, or temporary projects.'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'Use environment variables',
            description: 'Store the key and gateway address in the deployment environment. The example variable names can feed the SDK configuration directly.'
          }
        },
        callout: {
          title: 'Use a key per trust boundary',
          body: 'Frontend bundles, public repositories, build logs, and client errors can expose credentials. Browser applications should call the gateway through their own backend.'
        }
      },
      usage: {
        title: 'Usage and quota',
        description: 'Call GET /v1/usage to inspect the current key status, available quota, request totals, and model spending.',
        sections: {
          query: {
            title: 'Query usage',
            body: 'The usage endpoint authenticates directly with the API key and supports client status pages, balance monitoring, and automated alerts.',
            items: {
              first: {
                title: 'Authentication',
                description: 'Pass the target key in the Authorization: Bearer header. The endpoint works independently from dashboard sessions.'
              },
              second: {
                title: 'Daily detail',
                description: 'days accepts values from 1 through 90. timezone defines daily boundaries, such as Asia/Shanghai.'
              },
              third: {
                title: 'Model statistics range',
                description: 'start_date and end_date use YYYY-MM-DD and limit the aggregation period for model_stats.'
              }
            }
          },
          response: {
            title: 'Understand the response',
            body: 'The response presents quota mode or balance mode according to the key billing setup and includes available statistics.',
            items: {
              first: {
                title: 'mode and remaining',
                description: 'quota_limited indicates configured quota or rolling limits. unrestricted returns subscription or wallet balance information.'
              },
              second: {
                title: 'Usage totals',
                description: 'today and total include requests, input and output tokens, cache tokens, cost, RPM, TPM, and average latency.'
              },
              third: {
                title: 'Trends and model distribution',
                description: 'daily_usage supplies daily series. model_stats groups usage by model for cost allocation and capacity planning.'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'Query the latest 30 days',
            description: 'days controls the daily detail range and timezone controls date boundaries. The response also includes balance and lifetime totals.'
          }
        },
        callout: {
          title: 'Fields follow data availability',
          body: 'Quota, subscription, daily_usage, and model_stats appear according to key configuration and statistics availability. Give optional fields useful defaults in your client.'
        }
      },
      settings: {
        title: 'SDK configuration',
        description: 'Configure the base URL, timeout, retry policy, and streaming behavior for stable access from OpenAI-compatible clients.',
        sections: {
          connection: {
            title: 'Connection settings',
            body: 'Most compatible SDKs need a gateway API key and base URL. Existing business request objects can keep their protocol format.',
            items: {
              first: {
                title: 'Base URL',
                description: 'Use the current instance address ending in /v1 for OpenAI-compatible SDKs. The SDK appends each resource path.'
              },
              second: {
                title: 'Server-side credentials',
                description: 'Read the API key from server environment variables so credentials stay inside the trusted application tier.'
              },
              third: {
                title: 'Model source',
                description: 'Load /v1/models at startup or after configuration updates and use the returned id value for model.'
              }
            }
          },
          production: {
            title: 'Production guidance',
            body: 'Generation latency varies by task. Design timeout, retry, and cancellation behavior around the selected workload and streaming state.',
            items: {
              first: {
                title: 'Set an end-to-end timeout',
                description: 'Account for connection time and generation time. Long context, reasoning, and image requests often need a larger response timeout.'
              },
              second: {
                title: 'Bound automatic retries',
                description: 'Use exponential backoff for connection failures, 429 responses, and recoverable 5xx responses, with a clear retry limit.'
              },
              third: {
                title: 'Consume streams continuously',
                description: 'After the first event, keep consuming the protocol stream. User cancellation should abort the upstream request and release the connection.'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'TypeScript client configuration',
            description: 'This configuration targets the openai npm package. Tune timeouts and retry counts for the model type and service objective.'
          }
        },
        callout: {
          title: 'Handle partial streams at the application layer',
          body: 'Once a stream has emitted content, replaying the full request can produce duplicate output and billing. Surface this state for an explicit application decision.'
        }
      },
      authentication: {
        title: 'Authentication and errors',
        description: 'Gateway endpoints use API key authentication. The Bearer header offers broad compatibility across clients.',
        sections: {
          headers: {
            title: 'Authentication headers',
            body: 'The gateway accepts several compatible headers. Server integrations can standardize on Authorization: Bearer.',
            items: {
              first: {
                title: 'Authorization',
                description: 'Use Authorization: Bearer YOUR_API_KEY for the primary gateway endpoints under /v1.'
              },
              second: {
                title: 'Compatible headers',
                description: 'Anthropic-style clients can use x-api-key. Gemini clients can also use x-goog-api-key.'
              },
              third: {
                title: 'Request content type',
                description: 'Send application/json for JSON requests. Image editing uploads use multipart/form-data according to the Images protocol.'
              }
            }
          },
          errors: {
            title: 'Common status codes',
            body: 'Error responses include a type and message. Use the HTTP status to refresh credentials, adjust access, or delay a retry.',
            items: {
              first: {
                title: '401 authentication error',
                description: 'The key is missing, malformed, inactive, or unrecognized. Check the request header and the current key status.'
              },
              second: {
                title: '403 permission error',
                description: 'The key needs a group assignment or the group needs the target feature. Review the group, model, and feature switches.'
              },
              third: {
                title: '429 quota or concurrency limit',
                description: 'Quota, rolling limits, user concurrency, or image concurrency has reached capacity. Read Retry-After and apply backoff.'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'Send an authenticated request',
            description: 'The model list endpoint provides a quick connectivity check for the deployed key and group configuration.'
          }
        },
        callout: {
          title: 'Protect credentials in logs',
          body: 'Record request IDs, status codes, and error types while redacting Authorization, x-api-key, x-goog-api-key, and related credential fields.'
        }
      },
      chatCompletion: {
        title: 'Chat Completions API',
        description: 'A unified gateway endpoint for supported language models, designed for high-throughput and low-latency workloads.'
      },
      imageGeneration: {
        title: 'Image Generation API',
        description: 'Generate and edit images through OpenAI-compatible endpoints with the same API key group, quota, and scheduling rules.',
        sections: {
          generation: {
            title: 'Quick start',
            body: 'Image generation uses a JSON request. Available models, sizes, quality levels, and output formats follow upstream model capabilities.',
            items: {
              first: {
                title: 'Request endpoint',
                description: 'Send POST /v1/images/generations. model and prompt define the requested image capability and content.'
              },
              second: {
                title: 'Generation parameters',
                description: 'Pass size, quality, n, response_format, background, output_format, and related fields when the selected model supports them.'
              },
              third: {
                title: 'Read the result',
                description: 'Successful responses usually place b64_json or url in the data array and can include a revised_prompt value.'
              }
            }
          },
          editing: {
            title: 'Editing and runtime limits',
            body: 'Image editing and generation share authentication, billing, and scheduling. Editing also requires an account with the selected model capability.',
            items: {
              first: {
                title: 'Image editing',
                description: 'POST /v1/images/edits accepts a source image, prompt, and model. File uploads use multipart/form-data, with an optional mask for supported models.'
              },
              second: {
                title: 'Group capability',
                description: 'The Images API serves OpenAI and Grok groups with image generation enabled and a schedulable account for the requested model.'
              },
              third: {
                title: 'Concurrency and billing',
                description: 'Image tasks use dedicated concurrency slots and create usage records. A 429 response signals a quota or concurrency wait condition.'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'Generate an image',
            description: 'Confirm an available image model through /v1/models, then send a JSON request with size and quality values supported by that model.'
          },
          edit: {
            title: 'Edit an uploaded image',
            description: 'The edit endpoint uses multipart/form-data. image carries the local source file, and supported models can also accept mask, input_fidelity, background, and output_format.'
          },
          response: {
            title: 'Parse the result',
            description: 'For response_format=b64_json, decode data[].b64_json into an image. For response_format=url, download the file from data[].url promptly.'
          }
        },
        callout: {
          title: 'Check group access first',
          body: 'The group must enable image generation. Model access and a compatible upstream account complete the routing requirements for image requests.'
        }
      },
      videoGeneration: {
        title: 'Video Generation API',
        description: 'Call HappyHorse through an asynchronous video API for text-to-video, first-frame image-to-video, reference-to-video, and video editing.',
        sections: {
          capabilities: {
            title: 'HappyHorse capabilities and inputs',
            body: 'HappyHorse 1.0 and 1.1 each expose four capability-specific models. Image and video inputs use public HTTPS URLs reachable by the generation service.',
            items: {
              first: {
                title: 'Model names',
                description: 'Use happyhorse-1.0 or happyhorse-1.1 with the -t2v, -i2v, -r2v, or -video-edit suffix that matches the requested capability.'
              },
              second: {
                title: 'Media fields',
                description: 'Use first_frame for image-to-video, reference_images for reference-to-video, and video plus an optional image for video editing.'
              },
              third: {
                title: 'Generation parameters',
                description: 'resolution supports 720P and 1080P, ratio defaults to 16:9, and seconds or duration sets the output duration with a 5-second default.'
              }
            }
          },
          lifecycle: {
            title: 'Submit, poll, and settle',
            body: 'Video generation uses asynchronous tasks. The create endpoint returns a task ID immediately, while Sub2API polls the generation service and exposes a unified task status and result.',
            items: {
              first: {
                title: 'Create a task',
                description: 'POST /v1/videos creates a generation task, /v1/videos/generations is an alias, and video editing uses POST /v1/videos/edits.'
              },
              second: {
                title: 'Poll task status',
                description: 'Call GET /v1/videos/:id for queued, in_progress, settling, completed, or failed. A completed response includes video_url.'
              },
              third: {
                title: 'Billing and result URLs',
                description: 'Submission reserves Credits, success settles against actual output, and failure releases the reservation. expires_at identifies signed result URL expiry.'
              }
            }
          }
        },
        examples: {
          textToVideo: {
            title: 'Text to video',
            description: 'Choose a -t2v model and provide prompt. This 5-second 720P request is suitable for an initial connectivity check.'
          },
          imageToVideo: {
            title: 'Image to video from a first frame',
            description: 'Choose an -i2v model and provide the first-frame image URL through first_frame. image is also accepted as an alias.'
          },
          referenceToVideo: {
            title: 'Reference to video',
            description: 'Choose an -r2v model and provide one or more reference image URLs through the reference_images array.'
          },
          videoEdit: {
            title: 'Video editing',
            description: 'Choose a -video-edit model, provide the source video URL through video, and optionally add a reference image through image.'
          },
          status: {
            title: 'Poll a video task',
            description: 'Replace the path value with id from the create response. Poll every 5 to 10 seconds until the status reaches completed or failed.'
          }
        },
        callout: {
          title: 'Use publicly reachable media URLs',
          body: 'The generation service downloads resources referenced by first_frame, reference_images, video, and image. Keep each URL valid during execution and save generated output before expires_at.'
        }
      },
      modelList: {
        title: 'Model List API',
        description: 'Call GET /v1/models to retrieve the model catalog currently selectable by the API key.',
        sections: {
          catalog: {
            title: 'Response contents',
            body: 'The response follows the OpenAI list shape and derives its catalog from the key group platform and model configuration.',
            items: {
              first: {
                title: 'Isolated by API key',
                description: 'Separate keys can receive separate model catalogs at the same gateway address. The authenticated key always participates in catalog resolution.'
              },
              second: {
                title: 'Read data[].id',
                description: 'object is list, and each data entry contains an id suitable for the model field in later requests.'
              },
              third: {
                title: 'Platform-specific metadata',
                description: 'OpenAI and other compatible platforms can expose different metadata. Cross-platform clients can rely on id.'
              }
            }
          },
          refresh: {
            title: 'Refresh strategy',
            body: 'Account mappings, group custom lists, and platform defaults can update the catalog. Keep a refresh path in the client.',
            items: {
              first: {
                title: 'Use a short cache',
                description: 'Cache results briefly to reduce repeated requests and give users a manual refresh control.'
              },
              second: {
                title: 'Refresh after configuration changes',
                description: 'Fetch the catalog again after switching API keys, changing groups, or receiving model_not_found.'
              },
              third: {
                title: 'Availability changes over time',
                description: 'The catalog describes configured capabilities. Instant account capacity and provider health still affect individual requests.'
              }
            }
          }
        },
        examples: {
          primary: {
            title: 'Read the current catalog',
            description: 'Send the target API key with the request. Returned IDs can feed chat, Responses, or the corresponding capability endpoint.'
          }
        },
        callout: {
          title: 'Use the endpoint as your model source',
          body: 'Building the model picker from /v1/models lets clients follow administrator configuration and provider model updates automatically.'
        }
      }
    },
    parameterRows: {
      model: 'Required. The model ID to use; retrieve available values from the model list.',
      messages: 'Required. An array containing the current conversation messages.',
      temperature: 'Optional, defaults to 1. Sampling temperature from 0 through 2.',
      maxTokens: 'Optional. The maximum number of tokens generated for this completion.'
    }
  }
}
