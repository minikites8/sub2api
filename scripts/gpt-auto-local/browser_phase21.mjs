import fs from 'node:fs/promises'
import { chromium } from '/Users/user/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.mjs'
const env = Object.fromEntries((await fs.readFile('.env.gpt-auto-local','utf8')).split('\n').filter(x=>x&&!x.startsWith('#')).map(x=>{const n=x.indexOf('=');return [x.slice(0,n),x.slice(n+1)]}))
const base=`http://127.0.0.1:${env.LOCAL_HTTP_PORT||18080}`
const out='artifacts/gpt-auto-phase21'
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
await context.addInitScript(({token,user})=>{localStorage.setItem('auth_token',token);localStorage.setItem('auth_user',JSON.stringify(user));localStorage.setItem('sub2api_locale','en')},{token,user})
const checks=[]
const assert=(v,msg)=>{if(!v)throw new Error(msg)}
const expectText=async(locator,text)=>{await locator.getByText(text,{exact:false}).waitFor({timeout:15000})}
try {
 await page.goto(base+'/admin/gpt-auto?tab=pricing')
 await page.getByTestId('input_price_per_mtok').waitFor()
 for(const [id,value] of [['input_price_per_mtok','2'],['cached_input_price_per_mtok','0.5'],['output_price_per_mtok','8']])await page.getByTestId(id).fill(value)
 await page.getByTestId('preview-pricing').click()
 await page.getByTestId('confirm-pricing').waitFor()
 assert((await page.locator('table').innerText()).includes('$0.0057'),'example charge')
 assert(await page.getByTestId('save-pricing').isDisabled(),'save disabled before confirmation')
 await page.getByTestId('confirm-pricing').check()
 await page.getByTestId('save-pricing').click()
 await page.getByRole('status').filter({hasText:'Pricing saved'}).waitFor()
 await page.reload();await page.getByTestId('input_price_per_mtok').waitFor()
 assert(await page.getByTestId('input_price_per_mtok').inputValue()==='2','pricing reload')
 await page.getByTestId('preview-pricing').click();await page.getByTestId('confirm-pricing').waitFor()
 await page.screenshot({path:out+'/pricing-desktop.png',fullPage:true})
 checks.push('pricing entry, exact example, confirmation, save and reload')
 await page.goto(base+'/admin/groups')
 await page.locator('[data-tour="groups-create-btn"]').click()
 await page.locator('[data-tour="group-form-name"]').fill('phase21-ui-'+Date.now())
 await page.locator('[data-tour="group-form-platform"] button').click()
 await page.getByRole('option',{name:'OpenAI',exact:true}).click()
 const toggle=page.getByTestId('group-gpt-auto-toggle')
 await toggle.waitFor();assert(await toggle.getAttribute('aria-checked')==='false','create switch defaults off')
 const control=page.getByTestId('group-gpt-auto')
 await expectText(control,'Internal-only')
 await expectText(control,'Global live routing is off')
 await toggle.click()
 const dialog=page.getByRole('dialog')
 const name=await page.locator('[data-tour="group-form-name"]').inputValue()
 await dialog.locator('button[type="submit"]').click()
 await page.getByRole('dialog').waitFor({state:'hidden'})
 await page.reload()
 const row=page.getByRole('row').filter({hasText:name});await row.waitFor()
 await row.getByRole('button',{name:'Edit',exact:true}).click()
 await toggle.waitFor();assert(await toggle.getAttribute('aria-checked')==='true','edit saved state')
 await toggle.click()
 await page.getByRole('dialog').locator('button[type="submit"]').click()
 await page.getByRole('dialog').waitFor({state:'hidden'})
 checks.push('group create defaults off, enabled save, edit reload, revoke save')
 await page.goto(base+'/admin/gpt-auto?tab=pricing');await page.getByTestId('input_price_per_mtok').waitFor()
 await page.setViewportSize({width:390,height:844})
 await page.evaluate(()=>{document.documentElement.classList.add('dark')})
 await page.getByTestId('preview-pricing').click();await page.getByTestId('confirm-pricing').waitFor()
 await page.screenshot({path:out+'/pricing-mobile-dark.png',fullPage:true})
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1),'mobile viewport overflow')
 checks.push('mobile width and dark rendering')
 await context.addInitScript(()=>localStorage.setItem('sub2api_locale','zh'))
 await page.reload();await page.getByTestId('input_price_per_mtok').waitFor()
 assert((await page.getByTestId('gpt-auto-pricing').innerText()).includes('虚拟模型独立费率'),'Chinese pricing mode')
 await page.screenshot({path:out+'/pricing-mobile-zh.png',fullPage:true})
 checks.push('Chinese localization')
 assert(!errors.length,'browser errors: '+errors.join('; '))
 await fs.writeFile(out+'/browser-results.json',JSON.stringify({passed:true,checks,errors},null,2))
 console.log(JSON.stringify({passed:true,checks,errors}))
} catch(e) {
 await page.screenshot({path:out+'/browser-failure.png',fullPage:true})
 await fs.writeFile(out+'/browser-results.json',JSON.stringify({passed:false,checks,error:String(e),errors},null,2))
 throw e
} finally {await browser.close()}
