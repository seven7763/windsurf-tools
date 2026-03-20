"""单账号调试：Firebase → register_user → GetUserJwt。批量请用 tools/batch_quick_jwt.py

使用前设置环境变量（勿将真实密码提交到 Git）：
  set QUICK_KEY_EMAIL=...
  set QUICK_KEY_PASSWORD=...
可选：set QUICK_KEY_FIREBASE_WEB_API_KEY=...（默认与 Windsurf Web 客户端一致）
"""
import base64
import json
import os
import ssl
import sys
import urllib.request

sys.stdout.reconfigure(encoding="utf-8", errors="replace")

EMAIL = os.environ.get("QUICK_KEY_EMAIL", "").strip()
PASSWORD = os.environ.get("QUICK_KEY_PASSWORD", "").strip()
FK = os.environ.get("QUICK_KEY_FIREBASE_WEB_API_KEY", "AIzaSyDsOl-1XpT5err0Tcnx8FFod1H8gVGIycY").strip()

if not EMAIL or not PASSWORD:
    print("请设置环境变量 QUICK_KEY_EMAIL 与 QUICK_KEY_PASSWORD", file=sys.stderr)
    sys.exit(1)

ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

print(f"1. Firebase login: {EMAIL}")
url = f"https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key={FK}"
req = urllib.request.Request(
    url,
    data=json.dumps({"email": EMAIL, "password": PASSWORD, "returnSecureToken": True}).encode(),
    headers={"Content-Type": "application/json"},
)
data = json.loads(urllib.request.urlopen(req, context=ctx, timeout=15).read())
token = data["idToken"]
print(f"   token: {len(token)}B")

print("2. RegisterUser...")
req = urllib.request.Request(
    "https://api.codeium.com/register_user/",
    data=json.dumps({"firebase_id_token": token}).encode(),
    method="POST",
    headers={"Content-Type": "application/json"},
)
data = json.loads(urllib.request.urlopen(req, context=ctx, timeout=15).read())
api_key = data["api_key"]
name = data.get("name", "")
print(f"   Key: {api_key}")
print(f"   Name: {name}")

print("3. Verify...")
import grpc


def wv(v):
    p = []
    while v > 0x7F:
        p.append((v & 0x7F) | 0x80)
        v >>= 7
    p.append(v & 0x7F)
    return bytes(p)


def ps(fn, s):
    sb = s.encode()
    return wv((fn << 3) | 2) + wv(len(sb)) + sb


def pm(fn, d):
    return wv((fn << 3) | 2) + wv(len(d)) + d


def rv(data, pos):
    r = s = 0
    while pos < len(data):
        b = data[pos]
        pos += 1
        r |= (b & 0x7F) << s
        s += 7
        if not (b & 0x80):
            break
    return r, pos


def ppb(data):
    fields = []
    pos = 0
    while pos < len(data):
        tag, pos = rv(data, pos)
        if tag == 0:
            break
        fn = tag >> 3
        wt = tag & 7
        if wt == 0:
            val, pos = rv(data, pos)
            fields.append((fn, 0, val))
        elif wt == 2:
            l, pos = rv(data, pos)
            if pos + l > len(data):
                break
            fields.append((fn, 2, data[pos : pos + l]))
            pos += l
        elif wt == 5:
            fields.append((fn, 5, data[pos : pos + 4]))
            pos += 4
        elif wt == 1:
            fields.append((fn, 1, data[pos : pos + 8]))
            pos += 8
        else:
            break
    return fields


meta = ps(1, "windsurf") + ps(2, "1.48.2") + ps(3, api_key) + ps(7, "1.9566.11")
body = pm(1, meta)
ch = grpc.secure_channel("server.self-serve.windsurf.com:443", grpc.ssl_channel_credentials())
tmap = {0: "FREE", 1: "BASIC", 2: "ENTERPRISE", 3: "TEAM", 9: "TRIAL"}

try:
    r = ch.unary_unary(
        "/exa.auth_pb.AuthService/GetUserJwt",
        request_serializer=lambda x: x,
        response_deserializer=lambda x: x,
    )(body, metadata=[("authorization", api_key)], timeout=10)
    print(f"   JWT: OK ({len(r)}B)")
    for fn, wt, val in ppb(r):
        if wt == 2:
            try:
                s = val.decode()
                if s.startswith("eyJ") and len(s) > 100:
                    p = json.loads(base64.urlsafe_b64decode(s.split(".")[1] + "=="))
                    print(f"   teams_tier: {p.get('teams_tier', 'N/A')}")
                    print(f"   pro: {p.get('pro', 'N/A')}")
            except Exception:
                pass
except grpc.RpcError as e:
    print(f"   JWT: [{e.code().name}] {e.details()}")

try:
    r = ch.unary_unary(
        "/exa.seat_management_pb.SeatManagementService/GetUserStatus",
        request_serializer=lambda x: x,
        response_deserializer=lambda x: x,
    )(body, metadata=[("authorization", api_key)], timeout=10)
    print(f"   Status: OK ({len(r)}B)")
    for fn, wt, val in ppb(r):
        if fn == 1 and wt == 2:
            for sfn, swt, sval in ppb(val):
                if sfn == 7 and swt == 2:
                    try:
                        print(f"   email: {sval.decode()}")
                    except Exception:
                        pass
                elif sfn == 10 and swt == 0:
                    print(f"   tier: {sval} = {tmap.get(sval, 'UNKNOWN')}")
                elif sfn == 13 and swt == 2:
                    for pfn, pwt, pval in ppb(sval):
                        if pfn == 1 and pwt == 2:
                            for dfn, dwt, dval in ppb(pval):
                                if dfn == 2 and dwt == 2:
                                    try:
                                        print(f"   planName: {dval.decode()}")
                                    except Exception:
                                        pass
                        elif pfn == 4 and pwt == 0:
                            print(f"   available: {pval}")
                        elif pfn == 6 and pwt == 0:
                            print(f"   used: {pval}")
                        elif pfn == 8 and pwt == 0:
                            print(f"   premium: {pval}")
                        elif pfn == 9 and pwt == 0:
                            print(f"   flow: {pval}")
except grpc.RpcError as e:
    print(f"   Status: [{e.code().name}] {e.details()}")

ch.close()
print("\n=== RESULT ===")
print(f"Key: {api_key}")
print(f"Name: {name}")
