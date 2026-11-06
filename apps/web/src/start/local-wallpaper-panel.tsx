import { useEffect, useRef, useState } from 'react'
import { PushPinIcon } from '@phosphor-icons/react/dist/ssr/PushPin'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr/WarningCircle'
import { usePreferences } from '../preferences'
import { localMetadata, localThumbnail, type LocalFile } from './local-wallpapers'
import { useWallpaper } from './wallpaper-state'
import { formatBytes, formatDate, LibraryImage } from './wallpaper-library'
import styles from './wallpaper-library.module.scss'

export function LocalPanel({ selected, select }: { selected: LocalFile | null; select: (file: LocalFile) => void }) {
  const { local, preference, choose, shuffle } = useWallpaper(), { locale } = usePreferences(), zh=locale==='zh'
  const [query,setQuery]=useState(''),[term,setTerm]=useState(''),[page,setPage]=useState(1)
  const input=useRef<HTMLInputElement>(null)
  useEffect(()=>{void local.browse();return local.cancel},[local.browse,local.cancel])
  useEffect(()=>{const t=setTimeout(()=>{setTerm(query.toLocaleLowerCase());setPage(1)},300);return()=>clearTimeout(t)},[query])
  const filtered=local.files.filter(f=>f.path.toLocaleLowerCase().includes(term)), total=Math.max(1,Math.ceil(filtered.length/9)), currentPage=Math.min(page,total)
  const pinned=(file:LocalFile)=>preference?.source==='local' && preference.mode==='fixed' && preference.fixedPath===file.path
  const pin=(file:LocalFile)=>choose({version:1,source:'local',mode:pinned(file)?'random':'fixed',...(!pinned(file)?{fixedPath:file.path}:{})})
  const resume=()=>{if(preference?.source==='local' && preference.mode!=='off')shuffle()}
  const messages:Record<string,string>={permission:zh?'请重新授权或选择文件夹。':'Authorize or choose a folder.',scanning:zh?'正在读取文件列表…':'Reading file list…',limited:zh?'已达到扫描上限（2000 张 / 10000 个条目 / 16 层），请选较小的文件夹。':'Scan limit reached (2000 images / 10000 entries / 16 levels). Choose a smaller folder.',scanfailed:zh?'读取文件夹失败，请重新选择。':'Folder could not be read. Choose it again.'}
  return <div className={styles.panel}>
    <div className={styles.localTools}><input ref={el=>{input.current=el;el?.setAttribute('webkitdirectory','')}} data-local-folder type="file" multiple accept="image/jpeg,image/png,image/webp,image/avif" hidden onChange={e=>{if(e.target.files){local.fallback(e.target.files);resume()};e.target.value=''}} />
      <button onClick={async()=>{const picked=await local.pick();if(picked===false){input.current!.setAttribute('webkitdirectory','');input.current!.click()}else if(picked)resume()}}>{zh?'选择文件夹':'Choose folder'}</button>
      <button onClick={async()=>{if(await local.authorize())resume()}}>{zh?'重新授权':'Authorize'}</button><button onClick={()=>void local.scan().catch(()=>{})}>{zh?'刷新列表':'Refresh files'}</button>
      {!local.persistent && <p>{zh?'兼容模式：刷新后可能需要重新选择文件夹。':'Compatibility mode: select the folder again after reloading.'}</p>}
      {local.status && <p role="status">{messages[local.status]}</p>}
      <input className={styles.search} aria-label={zh?'搜索本地壁纸':'Search Local wallpapers'} placeholder={zh?'搜索文件名或相对路径':'Search filename or relative path'} value={query} onChange={e=>setQuery(e.target.value)} maxLength={128} />
    </div>
    <div className={styles.grid} aria-busy={local.status==='scanning'}>
      {local.status==='scanning' ? Array.from({length:9},(_,i)=><div key={i} className={styles.skeleton} />) : filtered.slice((currentPage-1)*9,currentPage*9).map(file=><div className={styles.card} key={file.path} data-selected={selected?.path===file.path}>
        <button className={styles.cardImage} aria-label={`${zh?'预览':'Preview'} ${file.name}`} onClick={()=>select(file)}><LocalThumbnail file={file} /><span className={styles.caption}>{file.name}</span></button>
        <button className={styles.pin} data-pinned={pinned(file)} aria-label={`${pinned(file)?zh?'取消固定':'Unpin':zh?'固定':'Pin'} ${file.name}`} aria-pressed={pinned(file)} onClick={()=>pin(file)}><PushPinIcon size={16} weight={pinned(file)?'fill':'regular'} /></button>
      </div>)}
      {!filtered.length && local.status!=='scanning' && <p>{zh?'尚无可浏览的本地图片。':'No local images to browse.'}</p>}
    </div>
    <footer className={styles.pagination}><button disabled={currentPage<=1} onClick={()=>setPage(currentPage-1)}>{zh?'上一页':'Previous'}</button><span>{currentPage} / {total}</span><button disabled={currentPage>=total} onClick={()=>setPage(currentPage+1)}>{zh?'下一页':'Next'}</button></footer>
  </div>
}
export function LocalThumbnail({ file, dimensions }: { file:LocalFile; dimensions?:(width:number,height:number)=>void }) {
  const [url,setURL]=useState(''),[failed,setFailed]=useState(false)
  useEffect(()=>{const abort=new AbortController();let current='';setURL('');setFailed(false)
    void localThumbnail(file,abort.signal).then(result=>{if(abort.signal.aborted){URL.revokeObjectURL(result.url);return};current=result.url;setURL(current);dimensions?.(result.width,result.height)}).catch(()=>{if(!abort.signal.aborted)setFailed(true)})
    return()=>{abort.abort();if(current)URL.revokeObjectURL(current)}
  },[file])
  return url?<LibraryImage key={url} src={url} />:failed?<span role="status"><WarningCircleIcon size={20} /></span>:<span className={styles.skeleton} />
}
export function LocalDetail({ file }: { file:LocalFile }) {
  const { preference,choose,shuffle }=useWallpaper(),{locale}=usePreferences(),zh=locale==='zh'
  const [fields,setFields]=useState<string[][]>([]),[size,setSize]=useState<number[]>([])
  useEffect(()=>{const abort=new AbortController();setFields([]);setSize([]);void localMetadata(file,abort.signal).then(setFields).catch(()=>{});return()=>abort.abort()},[file])
  const pinned=preference?.source==='local' && preference.mode==='fixed' && preference.fixedPath===file.path
  return <>
    <div className={styles.preview}><LocalThumbnail file={file} dimensions={(w,h)=>setSize([w,h])} /></div><h3>{file.name}</h3><p>{pinned?zh?'已固定':'Pinned':zh?'仅预览':'Preview only'}</p>
    <dl><dt>{zh?'相对路径':'Relative path'}</dt><dd>{file.path}</dd><dt>{zh?'格式 / 大小':'Format / Size'}</dt><dd>{file.name.split('.').at(-1)?.toUpperCase()} · {formatBytes(file.size)}</dd>{size.length>0&&<><dt>{zh?'分辨率':'Resolution'}</dt><dd>{size.join(' × ')}</dd></>}<dt>{zh?'修改时间':'Modified'}</dt><dd>{formatDate(new Date(file.modified).toISOString(),locale)}</dd>{fields.map(([key,value])=><div className={styles.metadataRow} key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl>
    <div className={styles.actions}><button onClick={()=>choose({version:1,source:'local',mode:pinned?'random':'fixed',...(!pinned?{fixedPath:file.path}:{})})}>{pinned?zh?'取消固定':'Unpin':zh?'固定壁纸':'Pin wallpaper'}</button><button onClick={()=>{choose({version:1,source:'local',mode:'random'});shuffle()}}>{zh?'使用本地随机':'Use Local random'}</button></div>
  </>
}
