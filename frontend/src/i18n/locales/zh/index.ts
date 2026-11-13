import gptQos from './gptQos'
import autoCredit from './autoCredit'
import accountPerformance from './accountPerformance'
import apiKeyModelPolicy from './apiKeyModelPolicy'
import lottery from './lottery'
import accountTestTiming from './accountTestTiming'
import openaiAuth from './openaiAuth'
import upstreamEntities from './upstreamEntities'
import profitOps from './profitOps'
import upstreamOps from './upstreamOps'
import channelOps from './channelOps'
import ipAudit from './ipAudit'
import traffic from './traffic'
import connections from './connections'
import securityRecords from './securityRecords'
import siwc from './siwc'
import priorityScheduling from './priorityScheduling'
import aiOps from './aiOps'
import aiOpsControl from './aiOpsControl'
import pelicanTests from './pelicanTests'
import modelTrace from './modelTrace'
import tokenGuardV2 from './tokenGuardV2'
import autoConfig from './autoConfig'
import qualityOps from './qualityOps'
import controlledExperiments from './controlledExperiments'
import gptAuto from './gptAuto'
import gptAutoHistory from './gptAutoHistory'
import accountOps from './accountOps'
import tokenGuard from './tokenGuard'
import adminAccessKeys from './adminAccessKeys'
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
import latencyObservability from './latencyObservability'

export default mergeMissingLocaleKeys(mergeMissingLocaleKeys({
  apiKeyModelPolicy,
  autoCredit,
 accountPerformance,
	lottery,
  accountTestTiming,
  openaiAuth,
  upstreamEntities,
  traffic,
  ipAudit,
  connections,
  securityRecords,
  siwc,
  autoConfig,
  priorityScheduling,
  gptQos,
  qualityOps,
  channelOps,
  upstreamOps,
  profitOps,
  controlledExperiments,
  gptAuto,
  gptAutoHistory,
  accountOps,
  aiOps,
  aiOpsControl,
  tokenGuard,
  pelicanTests,
  modelTrace,
  tokenGuardV2,
  requestTiming,
  latencyObservability,
  adminAccessKeys,
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
