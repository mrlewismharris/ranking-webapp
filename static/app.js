'use strict';
const $ = (s, el=document) => el.querySelector(s);
const toast = message => { const el=$('#toast'); el.textContent=message; el.classList.add('visible'); setTimeout(()=>el.classList.remove('visible'),2600); };
document.querySelectorAll('.share-button').forEach(button=>button.addEventListener('click',async()=>{const link=new URL(button.dataset.url,location.origin).href;try{await navigator.clipboard.writeText(link);toast('Link copied!');}catch{prompt('Copy this tournament link:',link);}}));
const dialog=$('#media-dialog');
const matchup=$('.matchup');
const comparisonVideos=[...document.querySelectorAll('.contender video')];
const volumeStatus=$('#volume-status');
let activeVideo=comparisonVideos[0]||null,expandedSource=null,volumeTimer=0,submitted=false;
let volume=1,muted=true;
try{
 const saved=JSON.parse(localStorage.getItem('comparison-sound')||'null');
 if(saved&&Number.isFinite(saved.volume)){volume=Math.min(1,Math.max(0,saved.volume));muted=saved.muted!==false;}
}catch{}
const allComparisonVideos=()=>[...comparisonVideos,...document.querySelectorAll('#expanded-media video')];
const pauseComparison=()=>allComparisonVideos().forEach(video=>video.pause());
const applySound=video=>{if(video.volume!==volume)video.volume=volume;if(video.muted!==muted)video.muted=muted;};
const saveSound=()=>{try{localStorage.setItem('comparison-sound',JSON.stringify({volume,muted}));}catch{}};
const setupSound=video=>{
 applySound(video);
 video.addEventListener('volumechange',()=>{
  // Ignore queued events from applying preferences to an inactive video.
  if(video!==(dialog?.open?$('#expanded-media video'):activeVideo))return;
  volume=video.volume;muted=video.muted;saveSound();allComparisonVideos().forEach(applySound);
 });
 video.addEventListener('play',()=>{
  if(video.paused||submitted||document.hidden){video.pause();return;}
  if(comparisonVideos.includes(video))activeVideo=video;
  allComparisonVideos().forEach(other=>{if(other!==video)other.pause();});
 });
};
const playComparison=video=>{
 if(!video||submitted||document.hidden)return;
 if(comparisonVideos.includes(video))activeVideo=video;
 allComparisonVideos().forEach(other=>{if(other!==video)other.pause();});
 applySound(video);
 video.play().catch(()=>{
  // Browsers can require muted autoplay until the first user interaction.
  if(video!==(dialog?.open?$('#expanded-media video'):activeVideo)||submitted||document.hidden)return;
  if(!video.muted){video.muted=true;video.play().catch(()=>{});}
 });
};
document.querySelectorAll('.expand-button').forEach(button=>button.addEventListener('click',()=>{
 const previous=$('#expanded-media video');
 if(previous&&expandedSource)expandedSource.currentTime=previous.currentTime;
 pauseComparison();
 expandedSource=$('video',button.closest('.contender'));
 const media=document.createElement(button.dataset.kind==='video'?'video':'img');
 media.src='/media/'+button.dataset.media;
 if(media.tagName==='VIDEO'){
  activeVideo=expandedSource;media.controls=true;media.playsInline=true;media.loop=true;setupSound(media);
  const position=expandedSource?.currentTime||0;
  media.addEventListener('loadedmetadata',()=>{media.currentTime=position;},{once:true});
 }else media.alt='Full-size contender';
 $('#expanded-media').replaceChildren(media);
 if(volumeStatus)dialog.append(volumeStatus);
 if(!dialog.open)dialog.showModal();
 if(media.tagName==='VIDEO')playComparison(media);
}));
if(dialog){
 $('.close-dialog',dialog).addEventListener('click',()=>dialog.close());
 dialog.addEventListener('click',event=>{if(event.target===dialog)dialog.close();});
 dialog.addEventListener('close',()=>{
  const media=$('#expanded-media video');
  if(media){if(expandedSource)expandedSource.currentTime=media.currentTime;media.pause();}
  $('#expanded-media').replaceChildren();expandedSource=null;
  if(volumeStatus)document.body.append(volumeStatus);
  playComparison(activeVideo);
 });
}

comparisonVideos.forEach(video=>{
 setupSound(video);
 const card=video.parentElement;
 const seek=document.createElement('div');seek.className='video-seek';
 const progress=document.createElement('progress');progress.max=1000;progress.value=0;progress.setAttribute('aria-hidden','true');
 const slider=document.createElement('input');slider.type='range';slider.min='0';slider.max='1000';slider.step='1';slider.value='0';slider.disabled=true;slider.setAttribute('aria-label','Seek video');
 seek.append(progress,slider);card.append(seek);
 const timestamp=seconds=>{const value=Math.max(0,Math.floor(seconds));return `${Math.floor(value/60)}:${String(value%60).padStart(2,'0')}`;};
 const update=()=>{const ready=Number.isFinite(video.duration)&&video.duration>0;slider.disabled=!ready;const value=ready?Math.min(1000,Math.max(0,video.currentTime/video.duration*1000)):0;progress.value=value;slider.value=String(value);slider.setAttribute('aria-valuetext',ready?`${timestamp(video.currentTime)} of ${timestamp(video.duration)}`:'Video loading');};
 let frame=0;
 const stop=()=>{cancelAnimationFrame(frame);frame=0;update();};
 const tick=()=>{update();if(!video.paused&&!video.ended)frame=requestAnimationFrame(tick);else frame=0;};
 video.addEventListener('play',()=>{card.classList.add('is-playing');if(!frame)frame=requestAnimationFrame(tick);});
 video.addEventListener('pause',()=>card.classList.remove('is-playing'));
 ['pause','ended','emptied','error'].forEach(event=>video.addEventListener(event,stop));
 ['loadedmetadata','durationchange','timeupdate','seeked'].forEach(event=>video.addEventListener(event,update));
 slider.addEventListener('input',()=>{if(Number.isFinite(video.duration)&&video.duration>0){video.currentTime=Number(slider.value)/1000*video.duration;update();}});
 video.addEventListener('click',()=>{if(video.paused)playComparison(video);else video.pause();});
 update();
});
if(matchup){
 matchup.addEventListener('submit',event=>{
  event.preventDefault();if(submitted||!event.submitter)return;
  submitted=true;pauseComparison();
  const button=event.submitter,chosen=button.closest('.contender');chosen.classList.add('selected');
  document.querySelectorAll('.contender').forEach(card=>{if(card!==chosen)card.classList.add('not-selected');});
  const hidden=document.createElement('input');hidden.type='hidden';hidden.name='winner';hidden.value=button.value;matchup.append(hidden);
  document.querySelectorAll('.vote-button').forEach(b=>b.disabled=true);
  setTimeout(()=>HTMLFormElement.prototype.submit.call(matchup),matchMedia('(prefers-reduced-motion: reduce)').matches?0:280);
 });
 document.addEventListener('keydown',event=>{
  if(event.defaultPrevented||event.ctrlKey||event.altKey||event.metaKey||event.target.isContentEditable||event.target.closest('input,textarea,select,[contenteditable="true"]'))return;
  if(!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown',' '].includes(event.key))return;
  event.preventDefault();
  if(submitted||event.repeat)return;
  if(event.key==='ArrowLeft'||event.key==='ArrowRight'){
   matchup.requestSubmit(matchup.querySelectorAll('.vote-button')[event.key==='ArrowLeft'?0:1]);
  }else if(event.key===' '){
   const next=comparisonVideos[(comparisonVideos.indexOf(activeVideo)+1)%comparisonVideos.length];
   if(!next)return;
   if(dialog.open)next.closest('.contender').querySelector('.expand-button').click();else playComparison(next);
  }else{
   const video=dialog.open?$('#expanded-media video'):activeVideo;
   if(!video)return;
   const change=event.key==='ArrowUp' ? 0.2 : -0.2;
   volume=Math.min(1,Math.max(0,Math.round((volume+change)*100)/100));
   muted=volume===0;allComparisonVideos().forEach(applySound);saveSound();
   volumeStatus.textContent=`Volume ${Math.round(volume*100)}%`;volumeStatus.hidden=false;
   clearTimeout(volumeTimer);volumeTimer=setTimeout(()=>volumeStatus.hidden=true,1400);
  }
 });
 document.addEventListener('visibilitychange',()=>{if(document.hidden)pauseComparison();});
 window.addEventListener('pagehide',pauseComparison);
 playComparison(activeVideo);
}
const form=$('#create-form');
if(form){
 const editing=form.dataset.editing==='true';
 let items=[...document.querySelectorAll('#existing-items span')].map(el=>({id:el.dataset.id,title:el.dataset.title,media:el.dataset.media,kind:el.dataset.kind})), pending=0, creating=false;
 let original=JSON.stringify(items);let dirty=false;
 form.addEventListener('input',()=>dirty=true);
 const error=message=>$('#form-error').textContent=message;
 const update=()=>{ $('#item-count').textContent=items.length;$('#create-button').disabled=creating||pending>0||items.length<2;$('#create-note').textContent=pending?`Processing ${pending} file${pending===1?'':'s'}… Videos may take a few minutes.`:items.length<2?'Add at least 2 choices.':`${items.length} choices.`;};
 function render(){const list=$('#items');list.replaceChildren();items.forEach(item=>{const row=document.createElement('div');row.className='item-row';let thumb=document.createElement(item.media?(item.kind==='video'?'video':'img'):'span');thumb.className='item-thumbnail';if(item.media){thumb.src='/media/'+item.media;if(item.kind==='video'){thumb.muted=true;thumb.preload='metadata';}else thumb.alt='Choice preview';}else thumb.textContent='Aa';row.append(thumb);const title=document.createElement('input');title.placeholder=item.media?'Name (optional)':'Choice name';title.setAttribute('aria-label','Choice name');title.maxLength=160;title.value=item.title;title.addEventListener('input',()=>item.title=title.value);row.append(title);const attach=document.createElement('button');attach.type='button';attach.className='item-media-button';attach.textContent=item.media?'Replace':'Add media';attach.addEventListener('click',()=>{const input=document.createElement('input');input.type='file';input.accept=document.getElementById("files").accept;input.addEventListener('change',()=>{if(input.files[0])upload(input.files[0],item);});input.click();});row.append(attach);const remove=document.createElement('button');remove.type='button';remove.className='remove-item';remove.textContent='×';remove.setAttribute('aria-label','Remove choice');remove.addEventListener('click',()=>{items=items.filter(i=>i!==item);render();});row.append(remove);list.append(row);});update();}
 async function upload(file,existing){if(creating)return;if(file.size>64*1024*1024){error(`${file.name}: maximum file size is 64 MB.`);return;}if(!existing&&items.length+pending>=256){error('A tournament can have up to 256 choices.');return;}pending++;update();try{const body=new FormData();body.append('file',file);const response=await fetch('/api/upload',{method:'POST',body});if(!response.ok)throw new Error(await response.text());const result=await response.json();if(existing){existing.media=result.id;existing.kind=result.kind;}else items.push({title:'',media:result.id,kind:result.kind});render();}catch(e){error(`${file.name}: ${e.message}`);}finally{pending--;update();}}
 async function files(list){error('');for(const file of list)await upload(file);}
 $('#browse').addEventListener('click',()=>$('#files').click());$('#files').addEventListener('change',event=>{files([...event.target.files]);event.target.value='';});const zone=$('#dropzone');['dragenter','dragover'].forEach(name=>zone.addEventListener(name,event=>{event.preventDefault();zone.classList.add('dragover');}));['dragleave','drop'].forEach(name=>zone.addEventListener(name,event=>{event.preventDefault();zone.classList.remove('dragover');}));zone.addEventListener('drop',event=>files([...event.dataTransfer.files]));
 const add=()=>{const input=$('#item-title');const title=input.value.trim();if(!title){input.focus();return;}if(items.length>=256){error('A tournament can have up to 256 choices.');return;}items.push({title,media:'',kind:'text'});input.value='';error('');render();input.focus();};$('#add-item').addEventListener('click',add);$('#item-title').addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();add();}});
 form.addEventListener('submit',async event=>{event.preventDefault();if(pending||creating)return;error('');if(items.some(i=>!i.title.trim()&&!i.media)){error('Add a name, image or video to each choice.');return;}creating=true;update();$('#create-button').textContent=editing?'Saving…':'Creating…';try{const response=await fetch(form.dataset.endpoint,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({version:Number(form.dataset.version),reset:$('#reset-scores')?.checked||false,creator:$('#creator').value,name:$('#name').value,description:$('#description').value,items})});if(!response.ok)throw new Error(await response.text());location.href=(await response.json()).url;}catch(e){error(e.message);creating=false;$('#create-button').textContent=editing?'Save changes':'Create tournament';update();}});
 render();
 window.addEventListener('beforeunload',event=>{if((pending||dirty||JSON.stringify(items)!==original)&&!creating){event.preventDefault();event.returnValue='';}});
}

// Connect saved match cards into a bracket, including placement-match branches.
const tree=$('#bracket-tree');
if(tree){
 const svg=$('.bracket-lines',tree);
 const draw=()=>{
  const bounds=tree.getBoundingClientRect();svg.setAttribute('width',String(tree.scrollWidth));svg.setAttribute('height',String(tree.scrollHeight));svg.replaceChildren();
  tree.querySelectorAll('.bracket-entry[data-source]').forEach(entry=>{
   if(!entry.dataset.source)return;
   const parent=document.getElementById(entry.dataset.source);
   const source=parent&&[...parent.querySelectorAll('.bracket-entry')].find(row=>row.dataset.item===entry.dataset.item);
   if(!source)return;
   const a=source.getBoundingClientRect(),b=entry.getBoundingClientRect();
   const x1=a.right-bounds.left,y1=a.top+a.height/2-bounds.top,x2=b.left-bounds.left,y2=b.top+b.height/2-bounds.top;
   const middle=(x1+x2)/2;
   const path=document.createElementNS('http://www.w3.org/2000/svg','path');
   path.setAttribute('d',`M ${x1} ${y1} H ${middle} V ${y2} H ${x2}`);
   path.setAttribute('class',source.classList.contains('picked')?'winner-path':'placement-path');svg.append(path);
  });
 };
 new ResizeObserver(draw).observe(tree);draw();
}

const slideshow=$('#slideshow-dialog');
if(slideshow){
 const slides=[...document.querySelectorAll('#slideshow-items span')].map(el=>({...el.dataset}));
 const stage=$('#slideshow-stage'),autoplay=$('#slideshow-autoplay'),previous=$('#previous-slide'),next=$('#next-slide'),status=$('#slideshow-status');
 let index=0,timer=0,generation=0,imageReady=false,muted=true;
 const clearTimer=()=>{clearTimeout(timer);timer=0;};
 const clearMedia=()=>{const video=$('video',stage);if(video){muted=video.muted;video.pause();video.removeAttribute('src');video.load();}stage.replaceChildren();};
 const play=video=>video.play().catch(()=>{if(slideshow.open&&stage.contains(video))status.textContent='Press play on the video to continue.';});
 const advanceSlide=()=>{
  if(!slideshow.open||!autoplay.checked)return;
  if(index<slides.length-1){show(index+1);return;}
  autoplay.checked=false;clearTimer();status.textContent='Slideshow finished.';
  const video=$('video',stage);if(video){video.loop=true;play(video);}
 };
 const scheduleImage=()=>{clearTimer();if(autoplay.checked&&imageReady&&!document.hidden&&slideshow.open)timer=setTimeout(advanceSlide,3000);};
 function show(position){
  clearTimer();generation++;const currentGeneration=generation;imageReady=false;clearMedia();index=position;status.textContent='';
  const slide=slides[index];$('#slide-title').textContent=slide.label;$('#slide-placement').textContent=slide.placement;$('#slide-counter').textContent=`${index+1} / ${slides.length}`;
  previous.disabled=index===0;next.disabled=index===slides.length-1;
  const media=document.createElement(slide.kind==='video'?'video':'img');
  media.addEventListener('error',()=>{if(currentGeneration!==generation)return;clearTimer();status.textContent='This media could not be loaded. Use the arrows to continue.';});
  if(slide.kind==='video'){
   media.controls=true;media.playsInline=true;media.muted=muted;media.loop=!autoplay.checked;media.preload='auto';
   media.addEventListener('ended',()=>{if(currentGeneration===generation)advanceSlide();});
  }else{
   media.alt=slide.label;
   media.addEventListener('load',()=>{if(currentGeneration!==generation)return;imageReady=true;scheduleImage();});
  }
  stage.append(media);media.src='/media/'+slide.media;
  if(slide.kind==='video')play(media);
 }
 $('#open-slideshow').addEventListener('click',()=>{slideshow.showModal();show(0);});
 $('.slideshow-close',slideshow).addEventListener('click',()=>slideshow.close());
 previous.addEventListener('click',()=>{if(index>0)show(index-1);});
 next.addEventListener('click',()=>{if(index<slides.length-1)show(index+1);});
 autoplay.addEventListener('change',()=>{
  clearTimer();status.textContent='';const video=$('video',stage);
  if(video){video.loop=!autoplay.checked;if(autoplay.checked)video.currentTime=0;play(video);}else scheduleImage();
 });
 slideshow.addEventListener('keydown',event=>{
  if(['INPUT','VIDEO','BUTTON'].includes(event.target.tagName))return;
  if(event.key==='ArrowLeft'&&index>0){event.preventDefault();show(index-1);}
  if(event.key==='ArrowRight'&&index<slides.length-1){event.preventDefault();show(index+1);}
 });
 slideshow.addEventListener('close',()=>{generation++;clearTimer();clearMedia();});
 document.addEventListener('visibilitychange',()=>{
  if(!slideshow.open)return;
  const video=$('video',stage);
  if(document.hidden){clearTimer();if(video)video.pause();}
  else if(video&&autoplay.checked)play(video);else scheduleImage();
 });
}
