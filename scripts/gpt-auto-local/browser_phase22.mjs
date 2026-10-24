import fs from 'node:fs/promises'
import { chromium } from '/Users/user/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.mjs'
const env = Object.fromEntries((await fs.readFile('.env.gpt-auto-local','utf8')).split('\n').filter(x=>x&&!x.startsWith('#')).map(x=>{const n=x.indexOf('=');return [x.slice(0,n),x.slice(n+1)]}))
const base=`http://127.0.0.1:${env.LOCAL_HTTP_PORT||18080}`
const out='artifacts/gpt-auto-phase22'
const browser=await chromium.launch({executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',headless:true})
const context=await browser.newContext({viewport:{width:1440,height:1100}})
const page=await context.newPage()
const errors=[];page.on('pageerror',e=>errors.push(e.message))
async function api(path,data,method='GET') {
 const response=await context.request.fetch(base+'/api/v1/'+path,{method,data,headers:token?{Authorization:'Bearer '+token}:{}})
 if(!response.ok())throw new Error(`${path}: ${response.status()} ${(await response.text()).slice(0,200)}`)
 return (await response.json()).data
}
let token=''
const login=await api('auth/login',{email:env.LOCAL_ADMIN_EMAIL,password:env.LOCAL_ADMIN_PASSWORD},'POST');token=login.access_token
const user=login.user || await api('auth/me')
await context.addInitScript(({token,user})=>{localStorage.setItem('auth_token',token);localStorage.setItem('auth_user',JSON.stringify(user));localStorage.setItem('sub2api_locale','en');localStorage.setItem(`admin_guide_${user.id}_${user.role}_v4_interactive`,'true')},{token,user})
const checks=[]
const assert=(v,msg)=>{if(!v)throw new Error(msg)}
const expectText=async(locator,text)=>{await locator.getByText(text,{exact:false}).waitFor({timeout:15000})}
try {
 await page.goto(base+'/admin/gpt-auto?tab=live')
 const toggle=page.getByTestId('dynamic-enabled')
 await toggle.waitFor()
 assert(await toggle.getAttribute('aria-checked')==='false','dynamic defaults off')
 const threshold=page.getByTestId('gpt-auto-dynamic').locator('input[max="20"]')
 await threshold.fill('0');await page.getByTestId('gpt-auto-live-save').click()
 await page.getByRole('alert').filter({hasText:'downgrade_stability_count'}).waitFor()
 await threshold.fill('2')
 checks.push('server-side dynamic threshold validation surfaced in UI')
 await toggle.click()
 await page.getByTestId('gpt-auto-live-save').click()
 await page.getByText('Live routing settings saved',{exact:true}).waitFor()
 await page.reload();await toggle.waitFor()
 assert(await toggle.getAttribute('aria-checked')==='true','dynamic persisted after reload')
 assert((await api('admin/gpt-auto/config')).live.live_enabled===false,'global live stays off')
 await toggle.click();await page.getByTestId('gpt-auto-live-save').click()
 await page.getByText('Live routing settings saved',{exact:true}).waitFor()
 checks.push('dynamic toggle off default, save/reload, rollback; global live remains off')
 await page.getByTestId('gpt-auto-dynamic').screenshot({path:out+'/dynamic-desktop.png',animations:'disabled'})
 await page.setViewportSize({width:390,height:844})
 await page.evaluate(()=>document.documentElement.classList.add('dark'))
 await page.addStyleTag({content:'*,*::before,*::after { transition: none !important; animation: none !important; scroll-behavior: auto !important; }'})
 await page.getByTestId('gpt-auto-dynamic').scrollIntoViewIfNeeded()
 await page.screenshot({path:out+'/dynamic-mobile-dark.png',animations:'disabled'})
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1),'no mobile overflow')
 checks.push('responsive mobile and dark rendering')
 await context.addInitScript(()=>localStorage.setItem('sub2api_locale','zh'))
 await page.reload();await toggle.waitFor()
 assert((await page.getByTestId('gpt-auto-dynamic').innerText()).includes('动态模型切换'),'Chinese localization')
 checks.push('English and Chinese labels')
 await page.setViewportSize({width:1440,height:1100})
 await page.goto(base+'/admin/gpt-auto?tab=logs')
 const evidence=JSON.parse(await fs.readFile(out+'/results.json','utf8'))
 const workflow=evidence.find(x=>x.name==='same_session_agent_workflow').detail.turns
 const requestID='client:'+workflow[workflow.length-1].request_id
 const matches=await api('admin/gpt-auto/decisions?request_id='+encodeURIComponent(requestID))
 const requestFilter=page.getByTestId('gpt-auto-decisions').locator('input[type="text"]').nth(1)
 await requestFilter.fill(requestID);await requestFilter.press('Enter')
 const decisionRow=page.getByTestId('gpt-auto-decision-'+matches.items[0].id)
 await decisionRow.waitFor();await decisionRow.click()
 await page.getByTestId('gpt-auto-detail').waitFor()
 assert(!(await page.getByTestId('gpt-auto-detail').innerText()).includes('假设推荐。该客户请求'),'live is not shadow')
 await page.getByTestId('dynamic-decision-detail').waitFor()
 await page.getByTestId('dynamic-decision-detail').locator('summary').click()
 assert((await page.getByTestId('dynamic-decision-detail').innerText()).includes('gpt-6-luna'),'bounded session timeline rendered')
 await page.screenshot({path:out+'/dynamic-detail.png',animations:'disabled'})
 checks.push('live decision detail is labelled as executed routing')
 assert(!errors.length,'browser errors: '+errors.join('; '))
 await fs.writeFile(out+'/browser-results.json',JSON.stringify({passed:true,checks,errors},null,2))
 console.log(JSON.stringify({passed:true,checks,errors}))
} catch(e) {
 await page.screenshot({path:out+'/browser-failure.png',fullPage:true})
 await fs.writeFile(out+'/browser-results.json',JSON.stringify({passed:false,checks,error:String(e),errors},null,2))
 throw e
} finally {
 const cfg=await api('admin/gpt-auto/config');cfg.classifier_api_key='';cfg.live.dynamic_routing.enabled=false;await api('admin/gpt-auto/config',cfg,'PUT')
 await browser.close()
}
