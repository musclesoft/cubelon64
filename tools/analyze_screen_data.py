#!/usr/bin/env python3
"""
Analyze the extracted screen data
"""
import socket
import time

def connect_vice():
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.settimeout(5)
    sock.connect(('localhost', 6510))
    return sock

def send_cmd(sock, cmd):
    sock.sendall((cmd + '\n').encode())
    time.sleep(0.05)
    response = b''
    try:
        while True:
            chunk = sock.recv(4096)
            if not chunk:
                break
            response += chunk
            if b'\n(C:' in response:
                break
    except:
        pass
    return response.decode('ascii', errors='ignore')

sock = connect_vice()

# Get first 40 bytes of screen (top row)
response = send_cmd(sock, "m 0400 0427")
print("Screen RAM $0400-$0427 (first row):")
print(response)
print()

# Get row 5 (where we draw character 0)
response = send_cmd(sock, "m 04c8 04ef")
print("Screen RAM $04C8-$04EF (row 5 - should be char 0):")
print(response)
print()

# Get row 6 (where we draw character 1)
response = send_cmd(sock, "m 04f0 0517")
print("Screen RAM $04F0-$0517 (row 6 - should be char 1):")
print(response)
print()

# Get row 7 (where we draw character 160/$A0)
response = send_cmd(sock, "m 0518 053f")
print("Screen RAM $0518-$053F (row 7 - should be char 160/$A0):")
print(response)
print()

# Check VIC registers
response = send_cmd(sock, "m d018 d018")
print("VIC_ADDR register ($D018):")
print(response)
print()

# Check first few chars of charset at $2000
response = send_cmd(sock, "m 2000 200f")
print("Charset at $2000 (first 16 bytes):")
print(response)

sock.close()
