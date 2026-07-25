import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import admin from './admin'
import misc from './misc'
import legacy from './legacy'
import docs from './docs'
import pricing from './pricing'
import { mergeMissingLocaleKeys } from '../mergeLegacy'

export default mergeMissingLocaleKeys({
  ...landing,
  ...common,
  ...dashboard,
  ...docs,
  ...pricing,
  admin,
  ...misc,
}, legacy)
