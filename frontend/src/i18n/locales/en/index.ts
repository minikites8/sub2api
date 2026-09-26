import qualityOps from './qualityOps'
import accountOps from './accountOps'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import legacy from './legacy'
import releaseAdditions from './releaseAdditions.json'
import { mergeMissingLocaleKeys } from '../mergeLegacy'

export default mergeMissingLocaleKeys(mergeMissingLocaleKeys({
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  qualityOps,
  accountOps,
  admin,
  ...misc,
}, releaseAdditions), legacy)
