package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		IP   string `yaml:"ip"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
}

type AudioServer struct {
	config     *Config
	mu         sync.RWMutex
	totalBytes uint64
	startTime  time.Time
	listeners  int
}

type Stats struct {
	Listeners int     `json:"listeners"`
	CPU       float64 `json:"cpu"`
	RAM       uint64  `json:"ram"`
	Bandwidth float64 `json:"bandwidth"`
}

func loadConfig() *Config {
	data, err := os.ReadFile("config.yml")
	if err != nil {
		cfg := &Config{}
		cfg.Server.IP = "0.0.0.0"
		cfg.Server.Port = 8080
		d, _ := yaml.Marshal(cfg)
		os.WriteFile("config.yml", d, 0644)
		return cfg
	}
	var cfg Config
	yaml.Unmarshal(data, &cfg)
	return &cfg
}

func NewAudioServer(cfg *Config) *AudioServer {
	return &AudioServer{
		config:    cfg,
		startTime: time.Now(),
	}
}

func (s *AudioServer) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	s.mu.Lock()
	s.listeners++
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.listeners--
		s.mu.Unlock()
	}()

	cmd := exec.Command("parec",
		"--format=s16le",
		"--rate=48000",
		"--channels=2",
		"--latency-msec=50",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, "Failed to start audio", 500)
		return
	}

	if err := cmd.Start(); err != nil {
		http.Error(w, "Failed to start audio", 500)
		return
	}
	defer cmd.Process.Kill()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", 500)
		return
	}

	buffer := make([]byte, 8192)
	for {
		n, err := stdout.Read(buffer)
		if err != nil || n == 0 {
			break
		}

		s.mu.Lock()
		s.totalBytes += uint64(n)
		s.mu.Unlock()

		if _, err := w.Write(buffer[:n]); err != nil {
			break
		}
		flusher.Flush()
	}
}

func getCPUUsage() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 5 {
		return 0
	}
	user, _ := strconv.ParseFloat(fields[1], 64)
	nice, _ := strconv.ParseFloat(fields[2], 64)
	system, _ := strconv.ParseFloat(fields[3], 64)
	idle, _ := strconv.ParseFloat(fields[4], 64)
	total := user + nice + system + idle
	used := user + nice + system
	if total == 0 {
		return 0
	}
	return (used / total) * 100
}

func getRAMUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc / 1024 / 1024
}

func (s *AudioServer) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte("pong"))
}

func (s *AudioServer) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	listeners := s.listeners
	bytes := s.totalBytes
	elapsed := time.Since(s.startTime).Seconds()
	s.mu.RUnlock()

	bandwidth := 0.0
	if elapsed > 0 {
		bandwidth = float64(bytes) / elapsed / 1024
	}

	stats := Stats{
		Listeners: listeners,
		CPU:       getCPUUsage(),
		RAM:       getRAMUsage(),
		Bandwidth: bandwidth,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(stats)
}

func (s *AudioServer) serveHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

func (s *AudioServer) servePlayer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(playerContent))
}

func (s *AudioServer) serveFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	faviconData := []byte{
		0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10, 0x00, 0x00, 0x01, 0x00, 0x20, 0x00, 0x68, 0x04,
		0x00, 0x00, 0x16, 0x00, 0x00, 0x00,
	}
	w.Write(faviconData)
}

const htmlContent = `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>soundsync</title>
<link rel="icon" href="/favicon.ico" type="image/x-icon">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{
font-family:monospace;
background:#000;
color:#fff;
padding:20px;
display:flex;
align-items:center;
justify-content:center;
min-height:100vh;
}
.c{
max-width:500px;
width:100%;
text-align:center;
}
h1{
font-size:32px;
margin-bottom:20px;
font-weight:normal;
}
.info{
background:#111;
border:1px solid #444;
padding:20px;
margin:20px 0;
font-size:14px;
line-height:1.6;
}
button{
background:#1db954;
color:#fff;
border:none;
padding:15px 40px;
font-size:18px;
font-weight:bold;
cursor:pointer;
border-radius:30px;
font-family:monospace;
transition:all 0.2s;
}
button:hover{
background:#1ed760;
transform:scale(1.05);
}
button:active{
transform:scale(0.98);
}
.note{
margin-top:20px;
opacity:0.6;
font-size:12px;
}
</style>
</head>
<body>
<div class="c">
<h1>soundsync</h1>
<div class="info">
<p>auto popup player when minimize or switch tabs</p>
<p style="margin-top:10px">click start to begin streaming</p>
</div>
<button id="startBtn">start</button>
<div class="note">player opens automatically in popup window</div>
</div>
<script>
let playerWindow=null;
let isPlaying=false;

function openPlayer(){
if(playerWindow&&!playerWindow.closed){
playerWindow.focus();
return playerWindow;
}
const w=400,h=600;
const left=(screen.width-w)/2;
const top=(screen.height-h)/2;
playerWindow=window.open(
'/player',
'soundsync_player',
'width='+w+',height='+h+',left='+left+',top='+top+',resizable=yes,scrollbars=no,status=no,toolbar=no,menubar=no,location=no'
);
return playerWindow;
}

document.getElementById('startBtn').onclick=()=>{
const pw=openPlayer();
if(pw){
isPlaying=true;
setTimeout(()=>{
if(pw&&!pw.closed){
pw.postMessage({action:'start'},'*');
}
},500);
}
};

document.addEventListener('visibilitychange',()=>{
if(document.hidden&&isPlaying){
openPlayer();
}
});

window.addEventListener('blur',()=>{
if(isPlaying){
setTimeout(()=>{
if(document.hidden||!document.hasFocus()){
openPlayer();
}
},100);
}
});

window.addEventListener('message',e=>{
if(e.data.action==='stopped'){
isPlaying=false;
}else if(e.data.action==='playing'){
isPlaying=true;
}
});
</script>
</body>
</html>`

const playerContent = `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>soundsync player</title>
<link rel="icon" href="/favicon.ico" type="image/x-icon">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{
font-family:monospace;
background:#000;
color:#fff;
overflow:hidden;
user-select:none;
}
.player{
height:100vh;
display:flex;
flex-direction:column;
}
.header{
background:#111;
border-bottom:1px solid #444;
padding:12px;
text-align:center;
}
.header h2{
font-size:16px;
font-weight:normal;
}
.main{
flex:1;
overflow-y:auto;
padding:12px;
}
.stats{
display:grid;
grid-template-columns:1fr 1fr;
gap:8px;
margin-bottom:12px;
}
.stat{
background:#111;
border:1px solid #444;
padding:10px;
}
.stat-label{
font-size:9px;
opacity:0.6;
margin-bottom:4px;
}
.stat-value{
font-size:14px;
}
.st{color:#0f0}
.chart{
height:80px;
background:#111;
border:1px solid #444;
margin-bottom:12px;
position:relative;
}
.chart-label{
position:absolute;
top:4px;
left:4px;
font-size:9px;
opacity:0.5;
z-index:10;
background:rgba(0,0,0,0.6);
padding:2px 4px;
}
canvas{width:100%;height:100%;display:block}
.controls{
padding:12px;
background:#111;
border-top:1px solid #444;
}
.ctrl{
margin-bottom:10px;
}
label{
display:block;
font-size:11px;
margin-bottom:4px;
}
input[type="range"]{
width:100%;
height:24px;
-webkit-appearance:none;
background:transparent;
cursor:pointer;
}
input[type="range"]::-webkit-slider-track{
background:#444;
height:3px;
}
input[type="range"]::-webkit-slider-thumb{
-webkit-appearance:none;
width:16px;
height:16px;
background:#1db954;
border-radius:50%;
cursor:pointer;
margin-top:-7px;
}
input[type="range"]::-moz-range-track{
background:#444;
height:3px;
}
input[type="range"]::-moz-range-thumb{
width:16px;
height:16px;
background:#1db954;
border:none;
border-radius:50%;
cursor:pointer;
}
button{
background:#1db954;
color:#fff;
border:none;
padding:12px;
font-size:14px;
font-weight:bold;
width:100%;
cursor:pointer;
border-radius:4px;
font-family:monospace;
}
button:active{
transform:scale(0.98);
}
button.stop{
background:#f00;
}
</style>
</head>
<body>
<div class="player">
<div class="header">
<h2>soundsync player</h2>
</div>
<div class="main">
<div class="stats">
<div class="stat">
<div class="stat-label">status</div>
<div class="stat-value st" id="st">stopped</div>
</div>
<div class="stat">
<div class="stat-label">listeners</div>
<div class="stat-value" id="listeners">0</div>
</div>
<div class="stat">
<div class="stat-label">buffer</div>
<div class="stat-value" id="buf">0</div>
</div>
<div class="stat">
<div class="stat-label">cpu</div>
<div class="stat-value" id="cpu">0%</div>
</div>
<div class="stat">
<div class="stat-label">ram</div>
<div class="stat-value" id="ram">0 MB</div>
</div>
<div class="stat">
<div class="stat-label">bandwidth</div>
<div class="stat-value" id="bw">0 KB/s</div>
</div>
<div class="stat">
<div class="stat-label">volume</div>
<div class="stat-value" id="voldisp">100%</div>
</div>
<div class="stat">
<div class="stat-label">ping</div>
<div class="stat-value" id="ping">0 ms</div>
</div>
</div>
<div class="chart">
<div class="chart-label">spectrum</div>
<canvas id="spectrum"></canvas>
</div>
<div class="chart">
<div class="chart-label">bass</div>
<canvas id="bass"></canvas>
</div>
</div>
<div class="controls">
<div class="ctrl">
<label>volume: <span id="vv">100</span>%</label>
<input type="range" id="vol" min="0" max="1000" value="100">
</div>
<div class="ctrl">
<label>bass: <span id="bb">0</span> dB</label>
<input type="range" id="bassCtrl" min="-10" max="20" value="0" step="1">
</div>
<button id="btn">start</button>
</div>
</div>
<script>
'use strict';

const specCanvas=document.getElementById('spectrum');
const bassCanvas=document.getElementById('bass');
const specCtx=specCanvas.getContext('2d',{alpha:false});
const bassCtx=bassCanvas.getContext('2d',{alpha:false});

function resizeCanvases(){
const dpr=window.devicePixelRatio||1;
const sr=specCanvas.getBoundingClientRect();
const br=bassCanvas.getBoundingClientRect();
specCanvas.width=sr.width*dpr;
specCanvas.height=sr.height*dpr;
bassCanvas.width=br.width*dpr;
bassCanvas.height=br.height*dpr;
specCtx.scale(dpr,dpr);
bassCtx.scale(dpr,dpr);
}
resizeCanvases();
window.addEventListener('resize',()=>setTimeout(resizeCanvases,100));

const st=document.getElementById('st');
const btn=document.getElementById('btn');
let ac,gain,bassFilter,an,running=false,reader,audioQueue=[];
let vol=1,bassVal=0;

document.getElementById('vol').oninput=e=>{
vol=e.target.value/100;
document.getElementById('vv').textContent=e.target.value;
document.getElementById('voldisp').textContent=e.target.value+'%';
if(gain)gain.gain.value=vol;
};

document.getElementById('bassCtrl').oninput=e=>{
bassVal=parseFloat(e.target.value);
document.getElementById('bb').textContent=e.target.value;
if(bassFilter)bassFilter.gain.value=bassVal;
};

btn.onclick=()=>{
if(running)stopStream();
else init();
};

function stopStream(){
running=false;
if(reader){
reader.cancel().catch(()=>{});
reader=null;
}
st.textContent='stopped';
st.classList.remove('st');
btn.textContent='start';
btn.classList.remove('stop');
audioQueue=[];
if(ac){
ac.close().then(()=>{
ac=null;
gain=null;
bassFilter=null;
an=null;
}).catch(()=>{});
}
if(window.opener){
window.opener.postMessage({action:'stopped'},'*');
}
if('mediaSession'in navigator){
navigator.mediaSession.playbackState='none';
}
}

function setupMediaSession(){
if('mediaSession'in navigator){
navigator.mediaSession.metadata=new MediaMetadata({
title:'soundsync stream',
artist:'live audio',
album:'pulseaudio'
});
navigator.mediaSession.setActionHandler('play',()=>{if(!running)init()});
navigator.mediaSession.setActionHandler('pause',stopStream);
navigator.mediaSession.setActionHandler('stop',stopStream);
navigator.mediaSession.playbackState='playing';
}
}

async function init(){
if(running)return;
try{
setupMediaSession();

ac=new(AudioContext||webkitAudioContext)({
sampleRate:48000,
latencyHint:'playback'
});
await ac.resume();

an=ac.createAnalyser();
an.fftSize=256;
an.smoothingTimeConstant=0.85;

bassFilter=ac.createBiquadFilter();
bassFilter.type='lowshelf';
bassFilter.frequency.value=200;
bassFilter.gain.value=bassVal;

gain=ac.createGain();
gain.gain.value=vol;

bassFilter.connect(gain);
gain.connect(an);
an.connect(ac.destination);

const res=await fetch('/stream');
if(!res.ok)throw new Error('stream failed');
reader=res.body.getReader();

st.textContent='playing';
st.classList.add('st');
btn.textContent='stop';
btn.classList.add('stop');
running=true;

if(window.opener){
window.opener.postMessage({action:'playing'},'*');
}

if('mediaSession'in navigator){
navigator.mediaSession.playbackState='playing';
}

let pending=new Uint8Array(0);
visualize();
processQueue();

while(running){
const{done,value}=await reader.read();
if(done||!running)break;

let combined=new Uint8Array(pending.length+value.length);
combined.set(pending);
combined.set(value,pending.length);

const chunkSize=4096;
while(combined.length>=chunkSize){
const chunk=combined.slice(0,chunkSize);
audioQueue.push(chunk);
let temp=new Uint8Array(combined.length-chunkSize);
temp.set(combined.slice(chunkSize));
combined=temp;
}
pending=combined;
}

if(running)stopStream();

}catch(e){
st.textContent='error';
st.classList.remove('st');
running=false;
btn.textContent='start';
btn.classList.remove('stop');
if(ac){
ac.close().catch(()=>{});
ac=null;
}
if('mediaSession'in navigator){
navigator.mediaSession.playbackState='none';
}
}
}

let nextPlayTime=0;

function processQueue(){
if(!running||!ac){
if(running)requestAnimationFrame(processQueue);
return;
}

if(ac.state==='suspended'){
ac.resume();
}

if(nextPlayTime===0||nextPlayTime<ac.currentTime){
nextPlayTime=ac.currentTime+0.05;
}

while(audioQueue.length>0&&nextPlayTime<ac.currentTime+0.2){
const data=audioQueue.shift();
const buf=createBuffer(data);
playBuffer(buf);
}

if(audioQueue.length>30){
audioQueue=audioQueue.slice(-20);
}

requestAnimationFrame(processQueue);
}

function createBuffer(data){
const samples=data.length/4;
const buf=ac.createBuffer(2,samples,48000);
const l=buf.getChannelData(0);
const r=buf.getChannelData(1);
for(let i=0;i<samples;i++){
const idx=i*4;
l[i]=((data[idx]|(data[idx+1]<<8))<<16>>16)/32768;
r[i]=((data[idx+2]|(data[idx+3]<<8))<<16>>16)/32768;
}
return buf;
}

function playBuffer(buf){
const src=ac.createBufferSource();
src.buffer=buf;
src.connect(bassFilter);
src.start(nextPlayTime);
nextPlayTime+=buf.duration;
}

function visualize(){
if(!running||!an){
if(running)requestAnimationFrame(visualize);
return;
}

const freqData=new Uint8Array(an.frequencyBinCount);
an.getByteFrequencyData(freqData);

const sr=specCanvas.getBoundingClientRect();
const br=bassCanvas.getBoundingClientRect();

specCtx.fillStyle='#000';
specCtx.fillRect(0,0,sr.width,sr.height);

const barW=sr.width/freqData.length;
for(let i=0;i<freqData.length;i++){
const h=(freqData[i]/255)*sr.height;
specCtx.fillStyle='#1db954';
specCtx.fillRect(i*barW,sr.height-h,barW-1,h);
}

const bassData=freqData.slice(0,10);
let bassAvg=0;
for(let v of bassData)bassAvg+=v;
bassAvg/=bassData.length;

bassCtx.fillStyle='#000';
bassCtx.fillRect(0,0,br.width,br.height);

const bassH=(bassAvg/255)*br.height;
bassCtx.fillStyle='#1db954';
bassCtx.fillRect(0,br.height-bassH,br.width,bassH);

document.getElementById('buf').textContent=audioQueue.length;
requestAnimationFrame(visualize);
}

let pingTime=0;
async function measurePing(){
const start=Date.now();
try{
await fetch('/ping');
pingTime=Date.now()-start;
}catch(e){
pingTime=0;
}
}

setInterval(async()=>{
try{
const r=await fetch('/stats');
const data=await r.json();
document.getElementById('listeners').textContent=data.listeners;
document.getElementById('cpu').textContent=data.cpu.toFixed(1)+'%';
document.getElementById('ram').textContent=data.ram+' MB';
document.getElementById('bw').textContent=data.bandwidth.toFixed(1)+' KB/s';
document.getElementById('ping').textContent=pingTime+' ms';
}catch(e){}
},1000);

setInterval(measurePing,2000);
measurePing();

window.addEventListener('message',e=>{
if(e.data.action==='start'){
if(!running)init();
}
});

window.addEventListener('beforeunload',e=>{
if(running){
stopStream();
}
});
</script>
</body>
</html>`

func main() {
	cfg := loadConfig()
	server := NewAudioServer(cfg)

	http.HandleFunc("/", server.serveHTML)
	http.HandleFunc("/player", server.servePlayer)
	http.HandleFunc("/stream", server.handleStream)
	http.HandleFunc("/stats", server.handleStats)
	http.HandleFunc("/ping", server.handlePing)
	http.HandleFunc("/favicon.ico", server.serveFavicon)

	addr := fmt.Sprintf("%s:%d", cfg.Server.IP, cfg.Server.Port)
	log.Printf("soundsync server on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
