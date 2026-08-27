#!/usr/bin/env python3
import json, socket, struct, sys, threading
from pathlib import Path

args=sys.argv[1:]
if 'version' in args:
    print('Xray 26.7.28 (L-UI test fake)')
    raise SystemExit(0)
try:
    cfg_path=Path(args[args.index('-config')+1])
except Exception:
    print('missing -config')
    raise SystemExit(2)
cfg=json.loads(cfg_path.read_text())
if '-test' in args:
    print('Configuration OK.')
    raise SystemExit(0)
ib=cfg['inbounds'][0]
port=int(ib['port'])
access=Path(cfg.get('log',{}).get('access','/tmp/fake-xray-access.log'))
outbound=(cfg.get('outbounds') or [{'tag':'direct'}])[0].get('tag','direct')
tag=ib.get('tag','test')

ls=socket.socket();ls.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);ls.bind(('127.0.0.1',port));ls.listen(20)

def relay(a,b):
    try:
        while True:
            d=a.recv(65536)
            if not d: break
            b.sendall(d)
    except Exception: pass
    finally:
        try:b.shutdown(socket.SHUT_WR)
        except Exception:pass

def handle(c):
    try:
        h=c.recv(2)
        if len(h)<2:return
        n=h[1]; c.recv(n); c.sendall(b'\x05\x00')
        h=c.recv(4)
        if len(h)<4:return
        atyp=h[3]
        if atyp==1: host=socket.inet_ntoa(c.recv(4))
        elif atyp==3:
            l=c.recv(1)[0]; host=c.recv(l).decode()
        elif atyp==4: host=socket.inet_ntop(socket.AF_INET6,c.recv(16))
        else:return
        dest_port=struct.unpack('!H',c.recv(2))[0]
        r=socket.create_connection((host,dest_port),timeout=5)
        c.sendall(b'\x05\x00\x00\x01\x00\x00\x00\x00\x00\x00')
        access.parent.mkdir(parents=True,exist_ok=True)
        with access.open('a') as f:f.write(f'from tcp:127.0.0.1 accepted tcp:{host}:{dest_port} [{tag} -> {outbound}]\n')
        t=threading.Thread(target=relay,args=(c,r),daemon=True);t.start();relay(r,c)
    finally:
        try:c.close()
        except Exception:pass

while True:
    c,_=ls.accept();threading.Thread(target=handle,args=(c,),daemon=True).start()
