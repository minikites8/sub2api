import priorityScheduling from './priorityScheduling'
import pelicanTests from './pelicanTests'
import tokenGuardV2 from './tokenGuardV2'
import autoConfig from './autoConfig'
import qualityOps from './qualityOps'
import controlledExperiments from './controlledExperiments'
import accountOps from './accountOps'
import tokenGuard from './tokenGuard'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import channelMonitorV3 from './channelMonitorV3'
import supportTickets from './supportTickets'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import legacy from './legacy'
import releaseAdditions from './releaseAdditions.json'
import { mergeMissingLocaleKeys } from '../mergeLegacy'

import requestTiming from './requestTiming'

export default mergeMissingLocaleKeys(mergeMissingLocaleKeys({
  autoConfig,
  priorityScheduling,
  qualityOps,
  controlledExperiments,
  accountOps,
  tokenGuard,
  pelicanTests,
  tokenGuardV2,
  requestTiming,
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...channelMonitorV3,
  ...supportTickets,
  ...batchImage,
  admin,
  ...misc,
}, releaseAdditions), legacy)
