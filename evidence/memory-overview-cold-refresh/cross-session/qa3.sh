#!/bin/bash
P=/Users/ran/.npm/_npx/d537ee5ee2a13f03/node_modules/playwright
S=/tmp/memoh-ovr-qa/shots3
PW=/tmp/memoh-ovr-qa/pw
send() { echo "$(date -u +%H:%M:%S) send $1"; node $PW/send.mjs $P "$S/$1.png" "$2" "$3" 2>&1 | tail -1; }
mem() { echo "$(date -u +%H:%M:%S) memfile $1"; node $PW/memfile.mjs $P "$S/$1.png" 2>&1 | tail -1; }
F='跨会话：F 第 1 轮'
send 01-f-turn1 '跨会话：F 第 1 轮，我喜欢爬山。' new
sleep 20
send 02-f-turn2 '跨会话：F 第 2 轮，我住在上海。' "$F"
sleep 30
send 03-g-turn1 '跨会话：G 第 1 轮，另开一个新会话。' new
mem 04-memory-md-after-g
sleep 10
send 05-f-turn3 '跨会话：F 第 3 轮，热会话继续。' "$F"
n=6
for i in 4 5 6 7 8 9 10 11; do
  sleep 225
  send "$(printf '%02d' $n)-f-turn$i" "跨会话：F 第 $i 轮，保持会话热。" "$F"
  n=$((n+1))
done
mem "$(printf '%02d' $n)-memory-md-after-max-lag"
echo "$(date -u +%H:%M:%S) done"
