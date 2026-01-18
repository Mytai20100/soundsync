# soundsync 
![Go](https://img.shields.io/badge/go-1.25-blue) 
![Version](https://img.shields.io/badge/version-0.1--alpha-lightgrey) 
---
<img src="https://github.com/Mytai20100/soundsync/blob/main/img/demo1.png" align="left" width="420"/> 

### What is soundsync? 
soundsync is an experimental project that streams **Ubuntu desktop audio to the web in real time**. The backend is written in **Golang**, capturing system audio and exposing it through a web interface for remote listening or testing. Use cases: 
- Stream Ubuntu audio to a browser <br clear="left"/> 

## Build Requirements: - Ubuntu - Go (recommended >= 1.24)
```bash
sudo apt update
sudo apt install -y pulseaudio-utils
git clone https://github.com/Mytai20100/soundsync.git
cd soundsync
go mod tidy 
go build -o soundsync
```
--- 
## Run
```bash
./soundsync
```
Open the browser and visit the server address printed in the terminal.
