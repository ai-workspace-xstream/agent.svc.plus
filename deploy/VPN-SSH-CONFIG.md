# 跳板机/VPN 配置指南

## 方案 1: 通过跳板机连接

### 配置 SSH Config
编辑 `~/.ssh/config`:

```
# 跳板机配置
Host jump-host
    HostName <跳板机IP或域名>
    User <跳板机用户>
    IdentityFile ~/.ssh/id_rsa
    ForwardAgent yes

# 目标主机通过跳板机连接
Host us-xhttp.svc.plus
    HostName 5.78.45.49
    User root
    IdentityFile ~/.ssh/id_rsa
    ProxyJump jump-host
```

### 测试连接
```bash
ssh us-xhttp.svc.plus "hostname && systemctl status agent-svc-plus"
```

### 执行部署
```bash
cd deploy/ansible
ansible-playbook playbooks/deploy_agent_svc_plus.yml -v
```

---

## 方案 2: 通过 VPN 连接

### WireGuard VPN 配置示例
编辑 `/etc/wireguard/wg0.conf`:

```ini
[Interface]
Address = 10.0.0.2/24
PrivateKey = <你的私钥>
DNS = 1.1.1.1

[Peer]
PublicKey = <服务器公钥>
AllowedIPs = 10.0.0.0/24, 5.78.45.49/32
Endpoint = <VPN服务器>:51820
PersistentKeepalive = 25
```

### 启动 VPN
```bash
# 启动 WireGuard
wg-quick up wg0

# 验证连接
ping 5.78.45.49
ssh root@5.78.45.49 "hostname"
```

### 执行部署
```bash
cd deploy/ansible
ansible-playbook playbooks/deploy_agent_svc_plus.yml -v
```

---

## 方案 3: SSH 隧道转发

### 创建 SSH 隧道
```bash
# 在本地创建隧道
ssh -L 2222:5.78.45.49:22 user@跳板机 -N -f

# 通过隧道连接
ssh -p 2222 root@localhost
```

### 配置 Ansible 使用隧道
编辑 `deploy/ansible/inventory.ini`:

```ini
[agent_svc_plus]
us-xhttp.svc.plus ansible_host=localhost ansible_port=2222 ansible_user=root

[all:vars]
ansible_ssh_private_key_file=~/.ssh/id_rsa
ansible_host_key_checking=False
```

---

## 方案 4: 使用 SSH ControlMaster

### 配置持久连接
编辑 `~/.ssh/config`:

```
Host *
    ControlMaster auto
    ControlPath /tmp/ansible-sockets/%r@%h-%p
    ControlPersist 60s
```

### 创建 socket 目录
```bash
mkdir -p /tmp/ansible-sockets
```

### 建立连接后执行
```bash
# 建立持久连接
ssh -MNf us-xhttp.svc.plus

# 执行 ansible
cd deploy/ansible
ansible-playbook playbooks/deploy_agent_svc_plus.yml -v
```

---

## 验证网络连通性

```bash
# 1. 检查 DNS 解析
dig us-xhttp.svc.plus +short
# 预期输出: 5.78.45.49

# 2. 检查端口连通性
nc -zv 5.78.45.49 22
# 或
nmap -p 22 5.78.45.49

# 3. 测试 SSH 连接
ssh -v root@5.78.45.49 "echo 'SSH OK'"

# 4. 测试 Ansible 连接
cd deploy/ansible
ansible agent_svc_plus -m ping
```

---

## 故障排除

### SSH 连接被拒绝
```bash
# 检查防火墙
ssh root@跳板机 "iptables -L -n | grep 22"

# 检查 SSH 服务
ssh root@跳板机 "systemctl status sshd"
```

### DNS 解析问题
```bash
# 添加 hosts 记录
echo "5.78.45.49 us-xhttp.svc.plus" | sudo tee -a /etc/hosts

# 或使用 IP 直接连接
ssh root@5.78.45.49
```

### Ansible 权限问题
```bash
# 使用 sudo
ansible-playbook playbooks/deploy_agent_svc_plus.yml -v --ask-become-pass

# 或配置 sudo 免密
echo "root ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/ansible
```
