import { apiClient } from '../client'

export interface GeneratedMediaStorageConfig {
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key?: string
  secret_configured?: boolean
  prefix: string
  public_base_url: string
  force_path_style: boolean
}

export interface GeneratedMediaStorageTestResult {
  ok: boolean
  message: string
}

export async function getConfig(): Promise<GeneratedMediaStorageConfig> {
  const { data } = await apiClient.get<GeneratedMediaStorageConfig>('/admin/generated-media-storage/config')
  return data
}

export async function updateConfig(config: GeneratedMediaStorageConfig): Promise<GeneratedMediaStorageConfig> {
  const { data } = await apiClient.put<GeneratedMediaStorageConfig>('/admin/generated-media-storage/config', config)
  return data
}

export async function testConnection(config: GeneratedMediaStorageConfig): Promise<GeneratedMediaStorageTestResult> {
  const { data } = await apiClient.post<GeneratedMediaStorageTestResult>('/admin/generated-media-storage/test', config)
  return data
}

export const generatedMediaStorageAPI = {
  getConfig,
  updateConfig,
  testConnection,
}

export default generatedMediaStorageAPI
