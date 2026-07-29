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
    videoModels: 'Video models',
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
    nav: {
      overview: 'Quick start',
      models: 'Choose a model',
      apiKeys: 'API keys',
      usage: 'Usage and quota',
      settings: 'Use an SDK',
      authentication: 'API key authentication',
      chatCompletion: 'Chat generation',
      imageGeneration: 'Image generation',
      videoGeneration: 'Video generation',
      modelList: 'Model list'
    },
    articles: {
      overview: {
        title: 'Quick start',
        description: 'With an API key and a model name, you can send your first chat request.',
        steps: {
          apiKey: {
            title: '1. Get an API key',
            description: 'Sign in and create a key on the API Keys page. Copy it when it is created and store it securely.',
            action: 'Go to API Keys'
          },
          model: {
            title: '2. Choose a model',
            description: 'Open the model catalog and copy the name of an available model.',
            action: 'View models'
          },
          request: {
            title: '3. Send a request',
            description: 'Replace the API key and model name in the example, then run the command. A model reply confirms that the setup works.'
          }
        }
      },
      models: {
        title: 'Choose a model',
        description: 'Find an available model and use its name in your request.',
        steps: {
          browse: {
            title: '1. Open the model catalog',
            description: 'The model catalog lists the available models and their main capabilities.',
            action: 'View models'
          },
          copy: {
            title: '2. Copy the model name',
            description: 'Choose a model for your task and copy its complete name.'
          },
          use: {
            title: '3. Add it to the request',
            description: 'Use the copied name as the model value.'
          }
        }
      },
      apiKeys: {
        title: 'API keys',
        description: 'An API key identifies your requests. Never publish it or commit it to a repository.',
        steps: {
          create: {
            title: '1. Create a key',
            description: 'Create a new key on the API Keys page in the dashboard.',
            action: 'Go to API Keys'
          },
          save: {
            title: '2. Save the key',
            description: 'The complete key is usually shown once, so store it securely right away.'
          },
          use: {
            title: '3. Use it in requests',
            description: 'Add the key to the Authorization header.'
          }
        }
      },
      usage: {
        title: 'Usage and quota',
        description: 'View request counts, spending, and remaining quota in the dashboard.',
        steps: {
          open: {
            title: '1. Open Usage',
            description: 'The Usage page summarizes requests for the current account.',
            action: 'View usage'
          },
          range: {
            title: '2. Choose a date range',
            description: 'Review usage by day, week, or a custom range.'
          },
          quota: {
            title: '3. Check remaining quota',
            description: 'Add funds or contact the administrator when the remaining quota is too low.'
          }
        }
      },
      settings: {
        title: 'Use an SDK',
        description: 'Use the OpenAI SDK by setting the API key and service URL.',
        steps: {
          install: {
            title: '1. Install the SDK',
            description: 'Install the openai package in your project.'
          },
          request: {
            title: '2. Configure and call',
            description: 'Replace the API key and model name in the example, then send the request.'
          }
        }
      },
      authentication: {
        title: 'API key authentication',
        description: 'Every API request must include a valid API key.',
        steps: {
          header: {
            title: '1. Add the authentication header',
            description: 'Use Authorization: Bearer YOUR_API_KEY.'
          },
          unauthorized: {
            title: '2. If authentication fails',
            description: 'For a 401 response, copy the key again and confirm that it is still enabled.'
          },
          limited: {
            title: '3. If requests are too frequent',
            description: 'For a 429 response, wait briefly before trying again.'
          }
        }
      },
      chatCompletion: {
        title: 'Chat generation',
        description: 'Send chat messages and receive a model response.',
        steps: {
          model: {
            title: '1. Choose a model',
            description: 'Copy the name of a chat-capable model from the model catalog.',
            action: 'View models'
          },
          request: {
            title: '2. Send a message',
            description: 'Set the API key, model name, and user message, then run the request.'
          },
          response: {
            title: '3. Read the reply',
            description: 'The assistant content in the result is the model reply.'
          }
        }
      },
      imageGeneration: {
        title: 'Image generation',
        description: 'Choose an image model, enter a prompt, and receive the generated result.',
        steps: {
          model: {
            title: '1. Choose an image model',
            description: 'Select a model that supports image generation.',
            action: 'View models'
          },
          request: {
            title: '2. Send a prompt',
            description: 'Replace the API key, model name, and prompt, then run the request.'
          },
          result: {
            title: '3. Save the image',
            description: 'The result can contain an image URL or Base64 data. Save it promptly.'
          }
        }
      },
      videoGeneration: {
        title: 'Video generation',
        description: 'Create asynchronous text-to-video, first-frame, reference-image, video editing, or single-image video tasks, then retrieve the result.',
        steps: {
          model: {
            title: 'Choose a video model',
            description: 'Copy a video model name from the model catalog. The examples below cover common video generation and editing workflows.',
            action: 'View models'
          },
          textToVideo: {
            title: 'Text to video',
            description: 'Use happyhorse-1.1-t2v and provide a prompt. This 5-second 720P request is suitable for an initial connectivity check.'
          },
          firstFrameToVideo: {
            title: 'Image to video from a first frame',
            description: 'Use happyhorse-1.1-i2v and provide a publicly reachable first-frame image URL through first_frame.'
          },
          referenceToVideo: {
            title: 'Reference to video',
            description: 'Use happyhorse-1.1-r2v and provide one or more reference image URLs through reference_images.'
          },
          videoEdit: {
            title: 'Video editing',
            description: 'Use happyhorse-1.1-video-edit, provide the source video URL through video, and optionally add a reference image through image.'
          },
          seedanceTextToVideo: {
            title: 'Text to video (low cost)',
            description: 'Use doubao-seedance-2-0-mini-260615 with a prompt. This 4-second 480P request is suitable for connectivity testing.'
          },
          seedanceImageToVideo: {
            title: 'Image to video from a first frame',
            description: 'Provide a publicly reachable JPG, JPEG, or PNG first-frame URL through first_frame.'
          },
          seedanceReferenceVideo: {
            title: 'Reference-video generation',
            description: 'Provide a publicly reachable reference video URL through video. The model follows its motion rhythm and visual content.'
          },
          veoTextToVideo: {
            title: 'Veo text to video',
            description: 'veo-3.1 and veo-3.1-fast support prompt-only generation. Base model names generate audio by default; add the -silent suffix to disable audio automatically, such as veo-3.1-fast-silent.'
          },
          veoHeadTailToVideo: {
            title: 'Veo first-and-last-frame video',
            description: 'Provide the first frame through image and the last frame through last_frame. All three Veo 3.1 models support this mode.'
          },
          veoReferenceToVideo: {
            title: 'Veo reference-image video',
            description: 'veo-3.1 and veo-3.1-fast accept one reference image through the reference_images array.'
          },
          klingTextToVideo: {
            title: 'Kling text to video',
            description: 'Use kling-v3 to generate 3-15 second videos with sound. Add the -silent suffix to disable sound, such as kling-v3-silent. mode supports std and pro.'
          },
          klingMultiShot: {
            title: 'Kling multi-shot video',
            description: 'kling-v3 and kling-v3-omni support custom shots. multi_prompt accepts up to 6 shots whose durations add up to seconds.'
          },
          klingHeadTailToVideo: {
            title: 'Kling first-and-last-frame video',
            description: 'Provide the first frame through image and the final frame through last_frame. kling-video-o1 supports 3-10 second first-and-last-frame generation.'
          },
          klingReferenceVideo: {
            title: 'Kling reference-video generation',
            description: 'kling-v3-omni accepts one reference video through reference_videos and supports 3-10 second output in this mode. The -silent suffix disables both output sound and the reference video audio.'
          },
          klingActionControl: {
            title: 'Kling motion control',
            description: 'Use kling-v3-action with a character image in image and a 3-10 second motion reference in video. character_orientation=image preserves the image orientation; video follows the reference orientation and supports up to 30 seconds. kling-v3-action-silent disables the reference video audio.'
          },
          result: {
            title: 'Get the result',
            description: 'Replace the path value with the id from the create response. Save the video at video_url when the task completes.'
          }
        }
      },
      modelList: {
        title: 'Model list',
        description: 'Use the API to get the model names available to the current API key.',
        steps: {
          request: {
            title: '1. Request the model list',
            description: 'Call /v1/models with the API key.'
          },
          read: {
            title: '2. Copy a model ID',
            description: 'Copy the required id from the returned data list.'
          },
          use: {
            title: '3. Use it in requests',
            description: 'Set the copied id as the model value in another request.'
          }
        }
      }
    }
  }
}
