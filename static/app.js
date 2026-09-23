'use strict';
const $ = (s, el=document) => el.querySelector(s);
const toast = message => { const el=$('#toast'); el.textContent=message; el.classList.add('visible'); setTimeout(()=>el.classList.remove('visible'),2600); };
document.querySelectorAll('.share-button').forEach(button=>button.addEventListener('click',async()=>{const link=new URL(button.dataset.url,location.origin).href;try{await navigator.clipboard.writeText(link);toast('Link copied!');}catch{prompt('Copy this tournament link:',link);}}));
const dialog=$('#media-dialog');
document.querySelectorAll('.expand-button').forEach(button=>button.addEventListener('click',()=>{const media=document.createElement(button.dataset.kind==='video'?'video':'img');media.src='/media/'+button.dataset.media;if(media.tagName==='VIDEO'){media.controls=true;media.autoplay=true;media.loop=true;}else{media.alt='Full-size contender';}$('#expanded-media').replaceChildren(media);dialog.showModal();}));
if(dialog){$('.close-dialog',dialog).addEventListener('click',()=>dialog.close());dialog.addEventListener('click',event=>{if(event.target===dialog)dialog.close();});dialog.addEventListener('close',()=>$('#expanded-media').replaceChildren());}
document.querySelectorAll('.contender video').forEach(video=>{
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
 video.addEventListener('play',()=>{if(!frame)frame=requestAnimationFrame(tick);});
 ['pause','ended','emptied','error'].forEach(event=>video.addEventListener(event,stop));
 ['loadedmetadata','durationchange','timeupdate','seeked'].forEach(event=>video.addEventListener(event,update));
 slider.addEventListener('input',()=>{if(Number.isFinite(video.duration)&&video.duration>0){video.currentTime=Number(slider.value)/1000*video.duration;update();}});
 card.addEventListener('mouseenter',()=>video.play().catch(()=>{}));
 card.addEventListener('mouseleave',()=>video.pause());
 video.addEventListener('click',()=>{if(video.paused)video.play().catch(()=>{});else video.pause();});
 update();
});
const matchup=$('.matchup');if(matchup){let submitted=false;matchup.addEventListener('submit',event=>{event.preventDefault();if(submitted||!event.submitter)return;submitted=true;const button=event.submitter;const chosen=button.closest('.contender');chosen.classList.add('selected');document.querySelectorAll('.contender').forEach(card=>{if(card!==chosen)card.classList.add('not-selected');});const hidden=document.createElement('input');hidden.type='hidden';hidden.name='winner';hidden.value=button.value;matchup.append(hidden);document.querySelectorAll('.crown-button').forEach(b=>b.disabled=true);setTimeout(()=>HTMLFormElement.prototype.submit.call(matchup),matchMedia('(prefers-reduced-motion: reduce)').matches?0:280);});}
const form=$('#create-form');
if(form){
 const editing=form.dataset.editing==='true';
 let items=[...document.querySelectorAll('#existing-items span')].map(el=>({id:el.dataset.id,title:el.dataset.title,media:el.dataset.media,kind:el.dataset.kind})), pending=0, creating=false;
 let original=JSON.stringify(items);let dirty=false;
 form.addEventListener('input',()=>dirty=true);
 const error=message=>$('#form-error').textContent=message;
 const update=()=>{ $('#item-count').textContent=items.length;$('#create-button').disabled=creating||pending>0||items.length<2;$('#create-note').textContent=pending?`Uploading ${pending} file${pending===1?'':'s'}…`:items.length<2?'Add at least 2 contenders to get started.':`${items.length} contenders. One crown.`;};
 function render(){const list=$('#items');list.replaceChildren();items.forEach(item=>{const row=document.createElement('div');row.className='item-row';let thumb=document.createElement(item.media?(item.kind==='video'?'video':'img'):'span');thumb.className='item-thumbnail';if(item.media){thumb.src='/media/'+item.media;if(item.kind==='video'){thumb.muted=true;thumb.preload='metadata';}else thumb.alt='Contender preview';}else thumb.textContent='Aa';row.append(thumb);const title=document.createElement('input');title.placeholder=item.media?'Add a name (optional)':'Contender name';title.setAttribute('aria-label','Contender name');title.maxLength=160;title.value=item.title;title.addEventListener('input',()=>item.title=title.value);row.append(title);const attach=document.createElement('button');attach.type='button';attach.className='item-media-button';attach.textContent=item.media?'Replace':'＋ Media';attach.addEventListener('click',()=>{const input=document.createElement('input');input.type='file';input.accept='image/jpeg,image/png,image/gif,image/webp,video/mp4';input.addEventListener('change',()=>{if(input.files[0])upload(input.files[0],item);});input.click();});row.append(attach);const remove=document.createElement('button');remove.type='button';remove.className='remove-item';remove.textContent='×';remove.setAttribute('aria-label','Remove contender');remove.addEventListener('click',()=>{items=items.filter(i=>i!==item);render();});row.append(remove);list.append(row);});update();}
 async function upload(file,existing){if(creating)return;if(file.size>64*1024*1024){error(`${file.name}: maximum file size is 64 MB.`);return;}if(!existing&&items.length+pending>=256){error('A tournament can have up to 256 contenders.');return;}pending++;update();try{const body=new FormData();body.append('file',file);const response=await fetch('/api/upload',{method:'POST',body});if(!response.ok)throw new Error(await response.text());const result=await response.json();if(existing){existing.media=result.id;existing.kind=result.kind;}else items.push({title:'',media:result.id,kind:result.kind});render();}catch(e){error(`${file.name}: ${e.message}`);}finally{pending--;update();}}
 async function files(list){error('');for(const file of list)await upload(file);}
 $('#browse').addEventListener('click',()=>$('#files').click());$('#files').addEventListener('change',event=>{files([...event.target.files]);event.target.value='';});const zone=$('#dropzone');['dragenter','dragover'].forEach(name=>zone.addEventListener(name,event=>{event.preventDefault();zone.classList.add('dragover');}));['dragleave','drop'].forEach(name=>zone.addEventListener(name,event=>{event.preventDefault();zone.classList.remove('dragover');}));zone.addEventListener('drop',event=>files([...event.dataTransfer.files]));
 const add=()=>{const input=$('#item-title');const title=input.value.trim();if(!title){input.focus();return;}if(items.length>=256){error('A tournament can have up to 256 contenders.');return;}items.push({title,media:'',kind:'text'});input.value='';error('');render();input.focus();};$('#add-item').addEventListener('click',add);$('#item-title').addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();add();}});
 form.addEventListener('submit',async event=>{event.preventDefault();if(pending||creating)return;error('');if(items.some(i=>!i.title.trim()&&!i.media)){error('Give each text contender a name, or add a photo or video.');return;}creating=true;update();$('#create-button').textContent=editing?'Saving…':'Creating…';try{const response=await fetch(form.dataset.endpoint,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({version:Number(form.dataset.version),reset:$('#reset-scores')?.checked||false,creator:$('#creator').value,name:$('#name').value,description:$('#description').value,items})});if(!response.ok)throw new Error(await response.text());location.href=(await response.json()).url;}catch(e){error(e.message);creating=false;$('#create-button').textContent=editing?'Save changes ↗':'Create tournament ↗';update();}});
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
  autoplay.checked=false;clearTimer();status.textContent='Countdown complete. You’ve reached your highest-ranked media item.';
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
