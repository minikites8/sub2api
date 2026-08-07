import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import legacy from './legacy'
import { mergeMissingLocaleKeys } from '../mergeLegacy'

export default mergeMissingLocaleKeys({
  ...landing,
  ...common,
  ...dashboard,
  ...batchImage,
  admin,
  ...misc,
}, legacy)
