# Maratona Full Cycle - Codelivery - Part 6 - Driver- Monitoramento em Produção

O projeto consiste em:

- Um sistema de monitoramento de veículos de entrega em tempo real.

Requisitos:

- Uma transportadora quer fazer o agendamento de suas entregas;
- Ela também quer ter o _feedback_ instantâneo de quando a entrega é realizada;
- Caso haja necessidade de acompanhar a entrega com mais detalhes, o sistema deverá informar, em tempo real, a localização do motorista no mapa.

#### Que problemas de negócio o projeto poderia resolver?

- O projeto pode ser adaptado para casos de uso onde é necessário rastrear e monitorar carros, caminhões, frotas e remessas em tempo real, como na logística e na indústria automotiva.

Dinâmica do sistema:

1. A aplicação _Order_ (_React_/_Nest.js_) é responsável pelas ordens de serviço (ou pedidos) e vai conter a tela de agendamento de pedidos de entrega. A criação de uma nova ordem de serviço começa o processo para que o motorista entregue a mercadoria;

2. A aplicação _Driver_ (_Go_) é responsável por gerenciar o contexto limitado de motoristas. Neste caso, sua responsabilidade consiste em disponibilizar os _endpoints_ de consulta;

3. Para a criação de uma nova ordem de serviço, a aplicação _Order_ obtém de _Driver_ os dados dos motoristas. Neste caso, _REST_ é uma opção pertinente, porque a comunicação deve ser a mais simples possível;

4. Após criar a nova ordem de serviço, _Order_ notifica a aplicação _Mapping_ (_Nest.js_/_React_) via _RabbitMQ_ de que o motorista deve iniciar a entrega. _Mapping_ é a aplicação que vai exibir no mapa a posição do motorista em tempo real. A aplicação _Simulator_ (_Go_) também é notificada sobre o início da entrega e começa a enviar para a aplicação _Mapping_ as posições do veículo;

5. Ao finalizar a entrega, a aplicação _Mapping_ notifica via _RabbitMQ_ a aplicação _Order_ de que o produto foi entregue e a aplicação altera o _status_ da entrega de Pendente para Entregue.

## Tecnologias

#### Operate What You Build

Nesta sexta versão, trabalhamos com tecnologias relacionadas aos processos de monitoramento aplicados a um _API Gateway_, operando com a _API_ da aplicação _Driver_.

- Backend

  - Golang

- API Gateway

  - Kong

- Monitoramento

  - Prometheus
  - EFK
    - Elasticsearch
    - Fluentd
    - Kibana

- GitOps Tool

  - ArgoCD

- Deploy

  - Kubernetes GKE

### O que faremos

Nesta parte do projeto, estamos nos aproximando mais de um ambiente de Produção, aonde integramos uma ferramenta de _API Gateway_, o _Kong API Gateway_, ao _cluster Kubernetes_. O _Kong_, além de desempenhar o seu papel de ponto único de entrada (_entrypoint_) na infraestrutura, roteando as chamadas para os respectivos serviços no _cluster_, também permite adicionar _plugins_ que servirão para:

1. Aplicar autenticação às rotas, utilizando o padrão _OpenID Connect_;
2. Aplicar _rate limiting_ às rotas;
3. Aplicar coleta de métricas e logs para monitorarmos o comportamento da aplicação e do próprio _API Gateway_ em Produção.

### Iniciando a infraestrutura

Vamos iniciar o provisionando um novo _cluster GKE_ para a aplicação _Driver_ a partir dos manifestos declarativos do _Terraform_.

Para isso, iremos aplicar o mesmo manifesto aplicado na [Parte 5](https://github.com/maratonafullcyclesidartaoss/fullcycle-maratona-1-codelivery-part-5-driver#terraform):

```
cd terraform

terraform apply
```

Por fim, não devemos esquecer de rodar o comando para configurar o _kubectl_ com as credenciais de acesso:

```
gcloud container clusters get-credentials $(terraform output -raw kubernetes_cluster_name) --region $(terraform output -raw region)
```

### Kubernetes Ingress versus Kong

Qual a diferença entre o _Kubernetes Ingress_ e um _API Gateway_?

O _Ingress_ é um ponto de entrada, expondo rotas nos protocolos _HTTP_ e _HTTPS_ para fora do cluster.

O papel do _Ingress_ é muito parecido com o papel do _API Gateway_, no entanto, o _Kubernetes Ingress_ contempla um propósito maior de servir como um ponto de entrada de rede, não somente de _APIs_, como é o caso do _API Gateway_. O _Ingress_ é ideal, por exemplo, no caso em que se faz a exposição de arquivos estáticos a partir de um _web server nginx_.

E como que o Kong funciona ao aplicar um manifesto com um objeto _Ingress_ no _Kubernetes_?

Quando um processo de _Continuous Delivery (CD)_, por exemplo, envia um objeto _Ingress_ para o _API Server_ do _Kubernetes_, ele vai validar se o objeto está íntegro, etc. Se estiver, ele vai disparar um evento para um objeto _Controller_ implementado pelo _Kong_, que vai avisar, baseado em uma marcação no objeto, se está ou não interessado nesse objeto. Se o _Controller_ do _Kong_ tiver interesse no ojeto, ele vai configurar o _Kong API Gateway_ com aquela rota que foi configurada no _Ingress_.

Ou seja, quando um objeto do tipo _Ingress_ sobe para o _Kubernetes_, se o _Kong_ entender que esse _Ingress_ é para ele, ele configura o _API Gateway_ com uma nova entrada.

### Instalando o Kong

Antes de instalar o _Kong API Gateway_, vamos criar uma imagem _Docker_ customizada do _Kong_ para adicionar 2 _plugins_: _OpenID Connect_ e _JWT2Header_.

```
$ mkdir infra
$ mkdir infra/kong-k8s
$ mkdir infra/kong-k8s/misc
$ mkdir infra/kong-k8s/misc/docker

$ touch infra/kong-k8s/misc/docker/Dockerfile
$ vim infra/kong-k8s/misc/docker/Dockerfile

FROM kong:3.0.2-alpine
USER root
ENV PACKAGES="openssl-devel kernel-headers gcc git openssh" \
    LUA_BASE_DIR="/usr/local/share/lua/5.1" \
    KONG_PLUGIN_OIDC_VER="1.2.4-4" \
    KONG_PLUGIN_COOKIES_TO_HEADERS_VER="1.1-4" \
    LUA_RESTY_OIDC_VER="1.7.5-1" \
    NGX_DISTRIBUTED_SHM_VER="1.0.7"

RUN set -ex \
    && apk --no-cache add libssl1.1 openssl curl unzip git \
    && apk --no-cache add --virtual .build-dependencies \
    make \
    gcc \
    openssl-dev \
    \
    ## Install plugins
    # Download ngx-distributed-shm dshm library
    && curl -sL https://raw.githubusercontent.com/grrolland/ngx-distributed-shm/${NGX_DISTRIBUTED_SHM_VER}/lua/dshm.lua > ${LUA_BASE_DIR}/resty/dshm.lua \
    # Remove old lua-resty-session
    && luarocks remove --force lua-resty-session \
    # Add Pluggable Compressors dependencies
    && luarocks install lua-ffi-zlib \
    && luarocks install penlight \
    # Build kong-oidc from forked repo because is not keeping up with lua-resty-openidc
    && curl -sL https://raw.githubusercontent.com/revomatico/kong-oidc/v${KONG_PLUGIN_OIDC_VER}/kong-oidc-${KONG_PLUGIN_OIDC_VER}.rockspec | tee kong-oidc-${KONG_PLUGIN_OIDC_VER}.rockspec | \
    sed -E -e 's/(tag =)[^,]+/\1 "master"/' -e "s/(lua-resty-openidc ~>)[^\"]+/\1 ${LUA_RESTY_OIDC_VER}/" > kong-oidc-${KONG_PLUGIN_OIDC_VER}.rockspec \
    && luarocks build kong-oidc-${KONG_PLUGIN_OIDC_VER}.rockspec \
    # Build kong-plugin-cookies-to-headers
    && curl -sL https://raw.githubusercontent.com/revomatico/kong-plugin-cookies-to-headers/master/kong-plugin-cookies-to-headers-${KONG_PLUGIN_COOKIES_TO_HEADERS_VER}.rockspec > kong-plugin-cookies-to-headers-${KONG_PLUGIN_COOKIES_TO_HEADERS_VER}.rockspec \
    # && luarocks build kong-plugin-cookies-to-headers-${KONG_PLUGIN_COOKIES_TO_HEADERS_VER}.rockspec \
    # Patch nginx_kong.lua for kong-oidc session_secret
    && TPL=${LUA_BASE_DIR}/kong/templates/nginx_kong.lua \
    # May cause side effects when using another nginx under this kong, unless set to the same value
    && sed -i "/server_name kong;/a\ \n\
set_decode_base64 \$session_secret \${{X_SESSION_SECRET}};\n" "$TPL" \
    # Patch nginx_kong.lua to set dictionaries
    && sed -i -E '/^lua_shared_dict kong\s+.+$/i\ \n\
variables_hash_max_size 2048;\n\
lua_shared_dict discovery \${{X_OIDC_CACHE_DISCOVERY_SIZE}};\n\
lua_shared_dict jwks \${{X_OIDC_CACHE_JWKS_SIZE}};\n\
lua_shared_dict introspection \${{X_OIDC_CACHE_INTROSPECTION_SIZE}};\n\
> if x_session_storage == "shm" then\n\
lua_shared_dict \${{X_SESSION_SHM_STORE}} \${{X_SESSION_SHM_STORE_SIZE}};\n\
> end\n\
    ' "$TPL" \
    # Patch nginx_kong.lua to add for memcached sessions
    && sed -i "/server_name kong;/a\ \n\
## Session:
set \$session_storage \${{X_SESSION_STORAGE}};\n\
set \$session_name \${{X_SESSION_NAME}};\n\
set \$session_compressor \${{X_SESSION_COMPRESSOR}};\n\
## Session: Memcached specific
set \$session_memcache_connect_timeout \${{X_SESSION_MEMCACHE_CONNECT_TIMEOUT}};\n\
set \$session_memcache_send_timeout \${{X_SESSION_MEMCACHE_SEND_TIMEOUT}};\n\
set \$session_memcache_read_timeout \${{X_SESSION_MEMCACHE_READ_TIMEOUT}};\n\
set \$session_memcache_prefix \${{X_SESSION_MEMCACHE_PREFIX}};\n\
set \$session_memcache_host \${{X_SESSION_MEMCACHE_HOST}};\n\
set \$session_memcache_port \${{X_SESSION_MEMCACHE_PORT}};\n\
set \$session_memcache_uselocking \${{X_SESSION_MEMCACHE_USELOCKING}};\n\
set \$session_memcache_spinlockwait \${{X_SESSION_MEMCACHE_SPINLOCKWAIT}};\n\
set \$session_memcache_maxlockwait \${{X_SESSION_MEMCACHE_MAXLOCKWAIT}};\n\
set \$session_memcache_pool_timeout \${{X_SESSION_MEMCACHE_POOL_TIMEOUT}};\n\
set \$session_memcache_pool_size \${{X_SESSION_MEMCACHE_POOL_SIZE}};\n\
## Session: DHSM specific
set \$session_dshm_region \${{X_SESSION_DSHM_REGION}};\n\
set \$session_dshm_connect_timeout \${{X_SESSION_DSHM_CONNECT_TIMEOUT}};\n\
set \$session_dshm_send_timeout \${{X_SESSION_DSHM_SEND_TIMEOUT}};\n\
set \$session_dshm_read_timeout \${{X_SESSION_DSHM_READ_TIMEOUT}};\n\
set \$session_dshm_host \${{X_SESSION_DSHM_HOST}};\n\
set \$session_dshm_port \${{X_SESSION_DSHM_PORT}};\n\
set \$session_dshm_pool_name \${{X_SESSION_DSHM_POOL_NAME}};\n\
set \$session_dshm_pool_timeout \${{X_SESSION_DSHM_POOL_TIMEOUT}};\n\
set \$session_dshm_pool_size \${{X_SESSION_DSHM_POOL_SIZE}};\n\
set \$session_dshm_pool_backlog \${{X_SESSION_DSHM_POOL_BACKLOG}};\n\
## Session: SHM Specific
set \$session_shm_store \${{X_SESSION_SHM_STORE}};\n\
set \$session_shm_uselocking \${{X_SESSION_SHM_USELOCKING}};\n\
set \$session_shm_lock_exptime \${{X_SESSION_SHM_LOCK_EXPTIME}};\n\
set \$session_shm_lock_timeout \${{X_SESSION_SHM_LOCK_TIMEOUT}};\n\
set \$session_shm_lock_step \${{X_SESSION_SHM_LOCK_STEP}};\n\
set \$session_shm_lock_ratio \${{X_SESSION_SHM_LOCK_RATIO}};\n\
set \$session_shm_lock_max_step \${{X_SESSION_SHM_LOCK_MAX_STEP}};\n\
" "$TPL" \
    # Patch kong_defaults.lua to add custom variables that are replaced dynamically in the template above when kong is started
    && TPL=${LUA_BASE_DIR}/kong/templates/kong_defaults.lua \
    && sed -i "/\]\]/i\ \n\
x_session_storage = cookie\n\
x_session_name = oidc_session\n\
x_session_compressor = 'none'\n\
x_session_secret = ''\n\
\n\
x_session_memcache_prefix = oidc_sessions\n\
x_session_memcache_connect_timeout = '1000'\n\
x_session_memcache_send_timeout = '1000'\n\
x_session_memcache_read_timeout = '1000'\n\
x_session_memcache_host = memcached\n\
x_session_memcache_port = '11211'\n\
x_session_memcache_uselocking = 'off'\n\
x_session_memcache_spinlockwait = '150'\n\
x_session_memcache_maxlockwait = '30'\n\
x_session_memcache_pool_timeout = '1000'\n\
x_session_memcache_pool_size = '10'\n\
\n\
x_session_dshm_region = oidc_sessions\n\
x_session_dshm_connect_timeout = '1000'\n\
x_session_dshm_send_timeout = '1000'\n\
x_session_dshm_read_timeout = '1000'\n\
x_session_dshm_host = hazelcast\n\
x_session_dshm_port = '4321'\n\
x_session_dshm_pool_name = oidc_sessions\n\
x_session_dshm_pool_timeout = '1000'\n\
x_session_dshm_pool_size = '10'\n\
x_session_dshm_pool_backlog = '10'\n\
\n\
x_session_shm_store_size = 5m\n\
x_session_shm_store = oidc_sessions\n\
x_session_shm_uselocking = off\n\
x_session_shm_lock_exptime = '30'\n\
x_session_shm_lock_timeout = '5'\n\
x_session_shm_lock_step = '0.001'\n\
x_session_shm_lock_ratio = '2'\n\
x_session_shm_lock_max_step = '0.5'\n\
\n\
x_oidc_cache_discovery_size = 128k\n\
x_oidc_cache_jwks_size = 128k\n\
x_oidc_cache_introspection_size = 128k\n\
\n\
" "$TPL" \
    ## Cleanup
    && rm -fr *.rock* \
    && apk del .build-dependencies 2>/dev/null \
    ## Create kong and working directory (https://github.com/Kong/kong/issues/2690)
    && mkdir -p /usr/local/kong \
    && chown -R kong:`id -gn kong` /usr/local/kong

RUN luarocks install kong-jwt2header
USER kong
```

Baseado nessa imagem, vamos prosseguir com a instalação do _Kong_ via _Helm_:

```
$ mkdir infra/kong-k8s/kong

$ touch infra/kong-k8s/kong/kong.sh
$ vim infra/kong-k8s/kong/kong.sh

#!/bin/bash
kubectl create ns kong
helm install kong kong/kong -f kong-conf.yaml --set proxy.type=LoadBalancer --set ingressController.installCRDs=false --set serviceMonitor.enabled=true --set serviceMonitor.labels.release=promstack --namespace kong

$ touch infra/kong-k8s/kong/kong-conf.yaml
$ vim infra/kong-k8s/kong/kong-conf.yaml

# Basic configuration for Kong without the ingress controller, using the Postgres subchart
# This installation does not create an Ingress or LoadBalancer Service for
# the Admin API. It requires port-forwards to access without further
# configuration to add them, e.g.:
# kubectl port-forward deploy/your-deployment-kong 8001:8001

image:
  repository: sidartasilva/kong
  tag: latest

env:
  prefix: /kong_prefix/
  database: "off"
  plugins: bundled,oidc,kong-jwt2header

admin:
  enabled: true
  http:
    enabled: true
    servicePort: 8001
    containerPort: 8001
  tls:
    parameters: []
  labels:
    enable-metrics: "true"

postgresql:
  enabled: false

ingressController:
  enabled: true
  installCRDs: false

podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8100"

proxy:
  type: LoadBalancer

autoscaling:
  enabled: true
  minReplicas: 1
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
```

Para instalar, basta executar o arquivo _kong.sh_:

```
$ ./kong.sh
```

A propriedade _proxy type_ refere-se ao tipo do serviço do _proxy_ relacionado ao _API Gateway_. Neste caso, optamos por _LoadBalancer_, porque vamos expor o _Kong_ na nossa infraestrutura de _cloud_.

Para verificar a criação dos objetos no _cluster_:

```
$ kubectl get all -n kong

NAME                             READY   STATUS    RESTARTS   AGE
pod/kong-kong-776dc6b568-ltsdw   2/2     Running   0          82s

NAME                                   TYPE           CLUSTER-IP     EXTERNAL-IP      PORT(S)                         AGE
service/kong-kong-admin                NodePort       10.7.254.22    <none>           8001:32347/TCP,8444:32522/TCP   84s
service/kong-kong-proxy                LoadBalancer   10.7.247.125   35.223.255.190   80:30164/TCP,443:30234/TCP      84s
service/kong-kong-validation-webhook   ClusterIP      10.7.241.87    <none>           443/TCP                         84s

NAME                        READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/kong-kong   1/1     1            1           83s

NAME                                   DESIRED   CURRENT   READY   AGE
replicaset.apps/kong-kong-776dc6b568   1         1         1       83s

NAME                                            REFERENCE              TARGETS         MINPODS   MAXPODS   REPLICAS   AGE
horizontalpodautoscaler.autoscaling/kong-kong   Deployment/kong-kong   <unknown>/80%   1         10        1          84s
```

### Ferramentas Adicionais

#### Prometheus

Neste momento, vamos fazer a instalação de algumas ferramentas adicionais para o _Kong_. Por exemplo, para a coleta de métricas, vamos utilizar o _Prometheus_:

```
$ mkdir infra/kong-k8s/misc/prometheus
$ touch infra/kong-k8s/misc/prometheus/prometheus.sh
$ vim infra/kong-k8s/misc/prometheus/prometheus.sh

#!/bin/bash
kubectl create ns monitoring
helm install prometheus-stack prometheus-community/kube-prometheus-stack --namespace monitoring
```

#### Keycloak

E, na seqüência, vamos instalar o _Keycloak_. E por que utilizar o _Keycloak_?

O _Keycloak_ é uma implementação certificada da especificação _OpenID Connect_. E o _OpenID Connect_ é uma definição que especifica como deve-se lidar com _tokens_, credenciais, etc., para fazer o controle do ciclo de vida de usuários e aplicações em relação à autenticação.

O _Kong_ conta com uma estratégia de controle de _tokens_, mas não é recomendado que se faça o controle de _tokens_ no _API Gateway_; é recomendado que se delegue essa responsabilidade para alguma implementação específica que lide com _tokens_ e que implemente o controle do ciclo de vida de usuários e da aplicação.

Além disso, vamos utilizar o _Keycloak_ por se tratar de uma ferramenta _open source_.

```
$ mkdir infra/kong-k8s/misc/keycloak

$ touch infra/kong-k8s/misc/keycloak/keycloak.sh

$ vim infra/kong-k8s/misc/keycloak/keycloak.sh

#!/bin/bash
kubectl create ns iam
helm install keycloak bitnami/keycloak --set auth.adminUser=keycloak,auth.adminPassword=keycloak --namespace iam
```

#### Backend APIs

E, neste momento, iremos configurar a nossa aplicação de _backend_. Um _API Gateway_ só faz sentido quando temos múltiplos _backends_, mas, neste caso, iremos aplicar os manifestos para apenas um serviço.

```
$ mkdir infra/kong-k8s/misc/apps

$ mkdir infra/kong-k8s/misc/apps/deployments
$ touch infra/kong-k8s/misc/apps/deployments/driver.yaml

apiVersion: apps/v1
kind: Deployment
metadata:
  name: driver
spec:
  selector:
    matchLabels:
      app: driver
  template:
    metadata:
      labels:
        app: driver
    spec:
      containers:
        - name: driver
          image: sidartasilva/fullcycle-maratona-1-codelivery-part-5-driver:latest
          resources:
            requests:
              cpu: "0.005"
              memory: 20Mi
            limits:
              cpu: "0.005"
              memory: 25Mi
          ports:
            - containerPort: 8081

$ mkdir infra/kong-k8s/misc/apps/services
$ touch infra/kong-k8s/misc/apps/services/driver.yaml

apiVersion: v1
kind: Service
metadata:
  annotations:
    ingress.kubernetes.io/service-upstream: "true"
  labels:
    app: driver
    stack: echo
    interface: rest
    language: golang
  name: driver
spec:
  type: LoadBalancer
  selector:
    app: driver
  ports:
    - name: http
      port: 80
      protocol: TCP
      targetPort: 8081


$ kubectl create ns driver
$ kubectl apply -f infra/kong-k8s/misc/apps --recursive -n driver
```

E, para conferir a criação dos objetos no _cluster_:

```
$ kubectl get all -n driver


NAME                          READY   STATUS    RESTARTS   AGE
pod/driver-6695fcc649-xwtsd   1/1     Running   0          49s

NAME             TYPE           CLUSTER-IP     EXTERNAL-IP    PORT(S)        AGE
service/driver   LoadBalancer   10.7.245.124   35.193.4.139   80:32489/TCP   49s

NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/driver   1/1     1            1           49s

NAME                                DESIRED   CURRENT   READY   AGE
replicaset.apps/driver-6695fcc649   1         1         1       50s
```

E, para conferir o retorno do serviço:

```
$ curl 35.193.4.139/drivers

{"Drivers":[{"uuid":"45688cd6-7a27-4a7b-89c5-a9b604eefe2f","name":"Wesley W"},{"uuid":"9a118e4d-821a-44c7-accc-fa99ac4be01a","name":"Luiz"}]}
```

### Kong Custom Resource Definitions

Neste momento, vamos fazer a configuração do _Kong_ a partir de objetos do _Kubernetes_.

Mas, como que, ao inputar objetos do _Kong_ no _cluster Kubernetes_, o _Kubernetes_ é capaz de entendê-los?

A partir de _Custom Resource Definitions (CRDs)_. Na prática, _CRDs_ são objetos do _Kong_ que estendem o formato de _API_ do _Kubernetes_. Lembrando que, para cada entidade do _Kong_, o _Kubernetes_, ao receber esse objeto, irá chamar o _Ingress Controller_ do _Kong_ para, então, configurar o objeto no _Kong_.

O primeiro _CRD_ que iremos aplicar é o de _KongPlugin_ para _rate limiting_:

```
$ mkdir infra/kong-k8s/misc/apis
$ touch infra/kong-k8s/misc/apis/kratelimit.yaml
$ vim infra/kong-k8s/misc/apis/kratelimit.yaml

apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: rl-by-header
config:
  second: 10000
  limit_by: header
  policy: local
  header_name: X-Credential-Identifier
plugin: rate-limiting

kubectl apply -f infra/kong-k8s/misc/apis/kratelimit.yaml -n driver
```

Neste caso, estamos configurando:

- Um _rate limiting_ de 10 mil _requests_ por segundo;
- Um atributo do _header_ (_limit_by: header_) para fazer o _rate limiting_;
- Uma política local (_policy: local_), ao invés de utilizar _Redis_, por exemplo, para fazer a contagem a partir de cada instância de _Kong_;
- O _header_name_ como _X-Credential-Identifier_. Ao utilizar-se o _plugin_ de _OpenID Connect_ da comunidade do _Kong_, esse _plugin_ cria o _header_, identificando o usuário do _token_. O _X-Credential-Identifier_, refere-se, portanto, a uma identificação de usuário. Neste caso, o _rate limiting_ está sendo feito por usuário, utilizando o valor do atributo _sub_ do _token_ _JWT_.

O _Kong_ conta também com a idéia de _plugins_ globais, onde as configurações do _plugin_ não têm escopo de _namespace_, logo, elas valem para todo o _cluster_.

Nesse sentido, iremos aplicar, agora, o _KongClusterPlugin_, onde utilizamos o _Prometheus_ para coletar as métricas em todas as rotas do _Kong_:

```
$ touch infra/kong-k8s/misc/apis/kprometheus.yaml
$ vim infra/kong-k8s/misc/apis/kprometheus.yaml

apiVersion: configuration.konghq.com/v1
kind: KongClusterPlugin
metadata:
  name: prometheus-driver
  annotations:
    kubernetes.io/ingress.class: "kong"
  labels:
    global: "true"
config:
  status_code_metrics: true
  latency_metrics: true
  upstream_health_metrics: true
  bandwidth_metrics: true
plugin: prometheus


$ kubectl apply -f infra/kong-k8s/misc/apis/kprometheus.yaml
```

### Kong Ingress

Quando falamos de _Ingress_, estamos nos referindo a algum endereço (_path_) para algum serviço, ou seja: roteamento.

Dessa forma, para fazer as configurações de rota, iremos, primeiramente, configurar um objeto _Ingress_ da _API_ do _Kubernetes_. Lembrando que o _Ingress_ tem escopo de _namespace_:

```
$ touch infra/kong-k8s/misc/apis/driver-api.yaml
$ vim infra/kong-k8s/misc/apis/driver-api.yaml

apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: driver-api
  annotations:
    kubernetes.io/ingress.class: "kong"
    konghq.com/override: do-not-preserve-host
    # konghq.com/plugins: oidc-driver,rl-by-header,prometheus-driver
spec:
  rules:
    - http:
        paths:
          - path: /api/driver
            pathType: Prefix
            backend:
              service:
                name: driver
                port:
                  number: 80



$ kubectl apply -f infra/kong-k8s/misc/apis/driver-api.yaml -n driver
```

Dessa forma, o _Ingress_ está fazendo a configuração para a rota _/api/driver_, aonde tudo que é prefixado com _/api/driver_ vai ser roteado para o _backend_ cujo serviço é _driver_ na porta 80.

E, em seguida, configuramos um objeto _KongIngress_ do _Kong_, o qual estende da _API_ do _Kubernetes_. Isso vai garantir que seja utilizado o mecanismo de _Services_ do _Kubernetes_:

```
$ touch infra/kong-k8s/misc/apis/king.yaml
$ vim infra/kong-k8s/misc/apis/king.yaml

apiVersion: configuration.konghq.com/v1
kind: KongIngress
metadata:
  name: do-not-preserve-host
route:
  preserve_host: false
  strip_path: true
upstream:
  host_header: driver.driver.svc
proxy:
  connect_timeout: 2000
  read_timeout: 2000
  write_timeout: 2000


$ kubectl apply -f infra/kong-k8s/misc/apis/king.yaml -n driver
```

As anotações e propriedades relacionadas a _do-not-preserve-host_, basicamente, definem que deve ser utilizado o mecanismo de _load balancing_ do _Kubernetes_, ao invés do _Kong_. Isso pode ser interessante quando se utiliza, juntamente, uma infraestrutura de _service mesh_, aonde o _service mesh_ vai poder utilizar o mecanismo de _Services_ do _Kubernetes_ para fazer o roteamentos, por exemplo, com _mTLS_.

Importante notar, também, a propriedade _strip_path_. Neste caso, setar essa propriedade como _true_ vai garantir que o _request_ seja roteado para o serviço de _backend_ (_upstream service_). A propriedade _strip_path_ é usada para remover o caminho definido no objeto _Ingress_ e encaminhar o _request_ para o serviço de _backend_ a partir do caminho restante; então, isso deve evitar um erro do tipo _404 page not found_, porque o caminho da _URL_ é diferente do caminho do serviço de _backend_.

Agora, já podemos verificar se a rota está funcionando:

```
$ kubectl get svc -n kong

NAME                           TYPE           CLUSTER-IP     EXTERNAL-IP      PORT(S)                         AGE
kong-kong-admin                NodePort       10.7.254.22    <none>           8001:32347/TCP,8444:32522/TCP   6h21m
kong-kong-proxy                LoadBalancer   10.7.247.125   35.223.255.190   80:30164/TCP,443:30234/TCP      6h21m
kong-kong-validation-webhook   ClusterIP      10.7.241.87    <none>           443/TCP

$ curl 35.223.255.190/api/driver/drivers
{"Drivers":[{"uuid":"45688cd6-7a27-4a7b-89c5-a9b604eefe2f","name":"Wesley W"},{"uuid":"9a118e4d-821a-44c7-accc-fa99ac4be01a","name":"Luiz"}]}
```

Isso mostra que a requisição está passando pelo _Kong_, que está roteando para o serviço de _driver_. Dessa forma, atingimos o objetivo de expor a rota para algum cliente que necessite acessar o _cluster_.

### Configuração do OpenID Connect Provider

Até este momento, fizemos a configuração da rota, mas, ainda não adicionamos nenhuma configuração de _plugin_ nessa rota. Assim, a rota permanece sem autenticação, por exemplo.

Então, o que faremos neste momento é adicionar adicionar um _plugin_ de autenticação, usando o padrão _OpenID Connect_. O _Kong_, por si só, não é uma implementação de _OpenID Connect_ e, entre as responsabilidade de um _API Gateway_, não está a de gerenciar o ciclo de vida dos usuários.

Assim, em geral, é utilizado uma ferramenta de _OpenID Connect_ para realizar o controle de usuários e aplicações. Em algumas empresas, essa ferramenta é chamada de _Identity Provider_. Dessa forma, nós já instalamos o _Keycloak_ como uma ferramenta adicional e, neste momento, iremos realizar algumas configurações na ferramenta.

Para acessar o painel administrativo do _Keycloak_, podemos fazer um _kubectl port-forward_:

```
$ kubectl port-forward svc/keycloak 8080:80 -n iam
```

Para logar: _Username: keycloak_ / _Password: keycloak_.

A primeira coisa que iremos fazer é criar um novo _realm_. O que vem a ser um _realm_? _Realm_ é uma divisão lógica de algum contexto de autenticação. Vamos nomear esse novo _realm_ como _driver_.

Em seguida, vamos criar os usuários. Primeiramente, vamos criar um usuário _maria_ com a credencial (i.e., setando a senha) _maria_, aonde a senha não deve ser temporária. Da mesma forma, criaremos um usuário _joao_ com a credencial _joao_.

Devemos lembrar que o _Kong_ sempre irá perguntar ao _Keycloak_ se o _token JWT_ (usando a especificação _OpenID Connect_) que está vindo na requisição é válido. Logo, precisamos criar também um cliente para o _Kong_ no _Keycloak_, já que o _Kong_ vai precisar se autenticar com o _Keycloak_. Então, vamos criar um novo cliente chamado _kong_, para que o _Kong_ - por meio do _plugin_ de _OpenID_ - possa validar o _token_. (O cliente _kong_ desempenha um papel de _client application_ normal do fluxo _OAuth 2_.)

Após criar o cliente _kong_, vamos acessar a aba _Credentials_ para obter o _client secret_. Essa credencial é importante, porque, sempre que o _Kong_ precisar executar algumas ações chamando _APIs_ do _OpenID Provider_, o _Kong_ vai se autenticar utilizando essa credencial.

### Configuração do Kong OpenID Connect Plugin

Verificamos que, até este momento, a nossa _API_ está exposta:

```
$ http://34.121.70.193/api/driver/drivers

{"Drivers":[{"uuid":"45688cd6-7a27-4a7b-89c5-a9b604eefe2f","name":"Wesley W"},{"uuid":"9a118e4d-821a-44c7-accc-fa99ac4be01a","name":"Luiz"}]}
```

Ou seja, as requisições estão sendo feitas sem autenticação. E esse não é o contexto desejado.

Sendo assim, vamos configurar o _plugin_ de autenticação do _Kong_. A primeira coisa que vamos fazer é copiar a credencial do cliente _kong_ no _Keycloak_ para o manifesto de _OpenID_ do _Kong_:

```
$ touch infra/kong-k8s/misc/apis/kopenid.yaml

apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: oidc-driver
config:
  client_id: "kong"
  client_secret: "uxQk57RtIqg5HWFO3EYf7oIFmk1uPuDs"
  discovery: "http://keycloak.iam/realms/driver/.well-known/openid-configuration"
  introspection_endpoint: "http://keycloak.iam/realms/driver/protocol/openid-connect/token/introspect"
  bearer_only: "yes"
  bearer_jwt_auth_enable: "yes"
  bearer_jwt_auth_allowed_auds:
    - account
plugin: oidc
```

É importante salientar que a especificação de _OpenID Connect Provider_ define que a implementação deve fornecer uma espécie de dicionário de métodos para se fazer a autenticação, para que sejam expostos todos os métodos que estão disponíveis para aquela implementação de _OpenID Connect_. Então, toda implementação de _OpenID Connect_ conta com um _endpoint_ padrão chamado de _well-known_. Nesse sentido, o _plugin_ de _OpenID_ do _Kong_ também define que é necessário informar uma _URL_ de _discovery_.

O _plugin_ também pode exigir que seja informado uma _URL_ de introspecção (_introspection_endpoint_), caso seja definido, na configuração do _plugin_, de que é necessário fazer instrospecção de _token_. O que isso significa? Um exemplo de introspecção é quando o _plugin_ do _Kong_ pergunta para a _URL_ de introspecção se o _token_ tem determinado escopo. Ou seja, significa obter informações a partir da _URL_ de introspecção acerca de um _token_.

No nosso caso, não iremos utilizar o mecanismo de introspecção, porque, então, a todo instante, será chamado o _Identity Provider (IDP)_, ou seja, o _Keycloak_, para validar o _token_.

No nosso caso, não é importante garantir tanta consistência: disponibilidade é mais importante. Então, optamos por fazer a autenticação baseado nas configurações de _bearer_only_ e _bearer_jwt_. O que isso significa? Significa que, quando o _plugin_ do _Kong_ for fazer a autenticação, o que ele vai fazer, no primeiro instante, é bater na _URL_ de _discovery_ e baixar a chave pública, a partir da propriedade _jwks_uri_, para validar a assinatura dos _tokens_, e essa chave pública ficará armazenada no _Kong_. Dessa forma, vamos evitar chamadas a todo instante para o _IDP_.

Assim, sempre que uma requisição chegar, ao invés de o _Kong_ ir até o _IDP_ para perguntar sobre o _token_, ele vai validar a assinatura do _token_ a partir da chave pública que foi armazenada no próprio _Kong_. Lembrando que, como estamos diminuindo a consistência, neste caso, para aumentar a disponibilidade, é importante termos, então, _tokens_ configurados com um tempo mais baixo de expiração.

Após aplicar o _plugin_ de _OpenID Connect_ do _Kong_:

```
$ kubectl apply -f  infra/kong-k8s/misc/apis/kopenid.yaml -n driver
```

, podemos verificar que a chamada para o serviço a partir do _Kong_ ainda continua exposta. Por quê?

Não devemos esquecer que é necessário, também, habilitar o _plugin_ na configuração do objeto _Ingress_:

```
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: driver-api
  annotations:
    kubernetes.io/ingress.class: "kong"
    konghq.com/override: do-not-preserve-host
    konghq.com/plugins: oidc-driver
spec:
  rules:
    - http:
        paths:
          - path: /api/driver
            pathType: Prefix
            backend:
              service:
                name: driver
                port:
                  number: 80


$ kubectl apply -f  infra/kong-k8s/misc/apis/driver-api.yaml -n driver
```

E, agora, percebemos um comportamento diferente:

```
$ curl http://34.121.70.193/api/driver/drivers

{
   "message":"Unauthorized"
}
```

O que significa que, se não for enviado um _token_ junto à requisição, o _Kong_ já retorna um erro _401 Unauthorized_.

Então, para gerarmos um novo _token_:

```
$ mkdir infra/kong-k8s/misc/token

$ touch infra/kong-k8s/misc/token/pod.yaml

apiVersion: v1
kind: Pod
metadata:
  name: testcurl
spec:
  containers:
    - name: curl
      image: curlimages/curl
      command: ["sleep", "600"]


$ touch infra/kong-k8s/misc/token/get-token.sh

#!/bin/bash
kubectl exec -it testcurl -- sh

curl --location --request POST 'http://keycloak.iam/realms/driver/protocol/openid-connect/token' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--data-urlencode 'client_id=kong' \
--data-urlencode 'grant_type=password' \
--data-urlencode 'username=maria' \
--data-urlencode 'password=maria' \
--data-urlencode 'client_secret=uxQk57RtIqg5HWFO3EYf7oIFmk1uPuDs' \
--data-urlencode 'scope=openid'

$ touch infra/kong-k8s/misc/token/apply-token.sh

#!/bin/bash
kubectl apply -f pod.yaml
```

Primeiramente, iremos criar um _POD_ no _namespace default_ para ser possível obter o _token_ a partir dele:

```
$ cd infra/kong-k8s/misc/token

$ ./apply-token.sh
```

E, em seguida, iremos nos conectar ao _POD_ para obter o _token_:

```
$ kubectl get po

NAME       READY   STATUS    RESTARTS   AGE
testcurl   1/1     Running   0          27s

$ kubectl exec -it testcurl -- sh

$ curl --location --request POST 'http://keycloak.iam/realms/driver/protocol/openid-connect/token' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--data-urlencode 'client_id=kong' \
--data-urlencode 'grant_type=password' \
--data-urlencode 'username=maria' \
--data-urlencode 'password=maria' \
--data-urlencode 'client_secret=uxQk57RtIqg5HWFO3EYf7oIFmk1uPuDs' \
--data-urlencode 'scope=openid'
```

Mas, por que é necessário obter o _token_ a partir de um _container_ no _Kubernetes_, se é possível gerar o token a partir da _interface_ do _Keycloak_? Porque, a partir da _interface_ do _Keycloak_, é gerado um _token_ com o valor _localhost:8080_ para a propriedade _iss_, que equivale ao _issuer_. Então, quando o _token_ bater no _plugin_ de _OpenID_ do _Kong_, ele vai verificar que é diferente de _keycloak.iam_ (_service.namespace_), conforme informado na propriedade _discovery_ e não vai autorizar.

Por fim, vamos copiar o _access token_ que foi gerado para utilizar na requisição:

```
$ curl -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICJkSU45a05MejdESWpCQjhnUWlMaGtvMDg1emRmSHpoTTZNVDVULXVaTkk4In0.eyJleHAiOjE2ODY0NDkzODcsImlhdCI6MTY4NjQ0OTA4NywianRpIjoiYjgyNDg4ZTEtMDNlZi00YjBkLWEwNmMtOTRlZGEzOGNmNGU5IiwiaXNzIjoiaHR0cDovL2tleWNsb2FrLmlhbS9yZWFsbXMvZHJpdmVyIiwiYXVkIjoiYWNjb3VudCIsInN1YiI6Ijg4Mzg1ZWM4LTJkMmQtNGRhZi05YmM0LTRkM2I2MDhmMzM4YyIsInR5cCI6IkJlYXJlciIsImF6cCI6ImtvbmciLCJzZXNzaW9uX3N0YXRlIjoiNjgwMTA0NmUtMTI1Yi00ODdiLWJhOWUtN2Y5MDYyODBmMGI0IiwiYWNyIjoiMSIsInJlYWxtX2FjY2VzcyI6eyJyb2xlcyI6WyJvZmZsaW5lX2FjY2VzcyIsImRlZmF1bHQtcm9sZXMtZHJpdmVyIiwidW1hX2F1dGhvcml6YXRpb24iXX0sInJlc291cmNlX2FjY2VzcyI6eyJhY2NvdW50Ijp7InJvbGVzIjpbIm1hbmFnZS1hY2NvdW50IiwibWFuYWdlLWFjY291bnQtbGlua3MiLCJ2aWV3LXByb2ZpbGUiXX19LCJzY29wZSI6Im9wZW5pZCBlbWFpbCBwcm9maWxlIiwic2lkIjoiNjgwMTA0NmUtMTI1Yi00ODdiLWJhOWUtN2Y5MDYyODBmMGI0IiwiZW1haWxfdmVyaWZpZWQiOmZhbHNlLCJwcmVmZXJyZWRfdXNlcm5hbWUiOiJtYXJpYSIsImdpdmVuX25hbWUiOiIiLCJmYW1pbHlfbmFtZSI6IiJ9.E4D2TyLnKHHOwtdoUawEngSd0j8kxqq_EwLp7NhyGN7geOqe3N0j2tppUyLAxmIjDwxm_kRng7V-JgsSHaOJ1O8Eibs_Wi92DMGZDpazXpQ0Gqst5NNzypl5T4f111bmMmz5Y7HQzO0BbqNqLA9A5_W1eau3pJYlDgmReTBQ_GjMsJLeOOdvM_UoBuwo99vnqnX8ksJXLT2zZoSAlMgB_lFneKRnK9LONZ5x-OddjKcwN31RAeARq5Fc_tN-g1TVhWi3ZTe1EC2vzyJBkTciiHXBQXRD1qUNADrVLKyS3RpHTB8HJeAcFs3Wbg7HS02j5XuEFeIw4dmplLw0xFUWkA" http://34.121.70.193/api/driver/drivers

$ {"Drivers":[{"uuid":"45688cd6-7a27-4a7b-89c5-a9b604eefe2f","name":"Wesley W"},{"uuid":"9a118e4d-821a-44c7-accc-fa99ac4be01a","name":"Luiz"}]}
```

Verificamos que a _API_ passa a estar protegida: neste momento, a _API_ passa a contar com autenticação utilizando o _OpenID Connect_ como especificação de segurança da aplicação.

### APIOps

Da mesma forma que o _GitOps_, o _APIOps_ se baseia no armazenamento das configurações - neste caso, de _APIs_ - em algum _storage_ versionado. O que isso significa?

Significa que, similarmente ao _GitOps_ (poderia-se convencionar como _API as Code_, da mesma forma que _Infrastructure as Code_ (_IaC_), talvez), só se pode tirar proveito do _APIOps_ a partir de arquivos declarativos. Por quê? Porque as ferramentas de automação só podem ser aplicadas sobre arquivos declarativos, da mesma forma que com _GitOps_.

Com relação ao processo de construção de uma nova _API_, ao iniciar esse processo, a primeira preocupação, normalmente, é em relação ao _design_. De forma geral, o _design_ é feito amparado por algum modelo de especificação de contratos, como o _OpenAPI_.

Em um segundo momento, então, esse contrato é armazenado em um ambiente versionado e, sempre que for necessário fazer uma mudança, é solicitado um _Pull Request_, aonde, dentro do processo de _CI_, podem ser feitas algumas validações para validar e testar o contrato. Após, é feito o _merge_ dessas alterações e segue-se o fluxo de _CD_ normalmente, passando-se pelo _GitOps Operator_ (_ArgoCD_), até chegar no _API Gateway_ (_Kong API Gateway_).

Na prática, o _APIOps_ nos ampara em manter algumas estruturas automatizadas dentro desse processo, por meio do uso de ferramentas.

O processo de _APIOps_ busca, basicamente, atender aos requisitos da empresa no que se refere a _APIs_. Então, ele auxilia em validar a conformidade do padrão de contrato, se as informações obrigatórias estão sendo passadas, etc., de forma a garantir que a _API_ esteja no padrão único da empresa. Além disso, o _APIOps_ permite fazer testes de contrato para garantir que as alterações na _API_ não estão quebrando o contrato.

Busca-se, no final, com o _APIOps_, aumentar a qualidade da _API_, para que ela seja disponibilizada de uma maneira uniforme para os clientes, aplicando-se um padrão de contrato, de maneira que eles possam ter uma experiência satisfatória quando forem integrar com a nossa _API_.

Em suma, o _APIOps_ se preocupa em:

- Armazenar e versionar todo o estado da _API_ no _Git_;
- Utilizar modelos de _Pull Requests_ para que as ferramentas apliquem: 1. Validações de conformidade do contrato; 2. Testes de contrato;

Nesse sentido, na próxima sessão, veremos como aplicar testes de contrato a partir do uso de ferramentas integradas no processo de _CI_.

### Checando contratos

Neste momento, vamos definir uma _suite_ de testes que representam a interação do cliente, ou seja, o que o cliente espera quando interage com a nossa _API_ e, para isso, iremos utilizar o mecanismo de testes do _Postman_:

```
// Validate status 2xx
pm.test("[GET]::/drivers - Status code is 200 OK", () => {
  pm.response.to.have.status(200);
});

// Validate if response header has matching content-type
pm.test("[GET]::/drivers - Content-Type is application/json", function () {
   pm.expect(pm.response.headers.get("Content-Type")).to.include("application/json");
});

// Validate if response has JSON Body
pm.test("[GET]::/drivers - Response has JSON Body", function () {
    pm.response.to.have.jsonBody();
});

// Response Validation
const schema = {"type":"array","items": [{"type": "object", "properties": {"uuid": {"type": "string"}, "name": {"type": "string"}}}]};

// Validate if response matches JSON schema
pm.test("[GET]::/drivers - Schema is valid", function() {
    pm.response.to.have.jsonSchema(schema);
});
```

Dessa forma, uma vez que seja feito um _GET_ em _/drivers_, espera-se que:

- No primeiro teste, o _status code_ do _response_ seja 200;
- No segundo teste, o _Content-Type_ no _header_ do _response_ seja _application/json_
- No terceiro teste, o corpo do _response_ contenha _JSON_;
- No quarto teste, seja validado o _schema_ do _JSON_, ou seja, o nome e o tipo das propriedades contidas no _JSON_ que compõe o corpo do _response_.

A idéia principal, aqui, é ter uma _suite_ de testes que representa o que o cliente espera da _API_ em termos de validação de contrato e em termos de resposta, sem considerar, neste caso, regras de negócio - apenas o _design_ da _API_, ou seja, o conjunto de elementos que formam o _response_ de determinada requisição.

Agora, como automatizar essa _suite_ de testes? O _Postman_ permite exportar essa coleção de testes em formato _JSON_ e, a partir disso, é possível utilizar uma ferramenta do próprio _Postman_ chamada _[Newman](https://learning.postman.com/docs/collections/using-newman-cli/installing-running-newman/)_ para rodar a _collection_ de testes dentro do processo de _CI_:

```
      - name: Install Global Dependencies
        if: steps.cache.outputs.cache-hit != 'true'
        run: npm -g install @stoplight/prism-cli newman

      - name: Run contract tests
        run: |
          prism mock docs/swagger.yaml & sleep 2 && newman run docs/driver.postman.json
```

Conjuntamente ao _Newman_, iremos utilizar uma ferramenta de _mocking_ chamada _[Prism](https://docs.stoplight.io/docs/prism/83dbbd75532cf-http-mocking)_. O _Prism_ vai expor a rota baseado na estrutura de contrato da _API_ (i.e., o arquivo _yaml_ em formtato _OpenAPI_), aonde são definidos os objetos de _request_ e _response_.

Com isso, é possível verificar se o _design_ do contrato, exposto pelo _Prism_, está aderente ao que o cliente espera. E a definição de o que o cliente espera é contemplada no teste de contrato que o _Newman_ executa.

Então, se for feita alguma alteração na estrutura do contrato que não reflita o que o cliente espera, o teste do contrato vai acusar um problema.

### Destruindo a infraestrutura

Chegou o momento de liberar os recursos no ambiente _cloud_.

Para tanto, iremos utilizar o _Terraform_, que irá remover todos os recursos relacionados com o _cluster GKE_.

```
cd terraform/

terraform destroy
```

![Cluster GKE destruído](./images/cluster-gke-destruido.png)

#### Referências

UDEMY. Como implementar GitFlow en Gitlab y Github. 2023. Disponível em: <https://www.udemy.com/course/como-implementar-gitflow-en-gitlab-y-github>. Acesso em: 26 mai. 2023.

FULL CYCLE 3.0. Integração contínua. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 26 mai. 2023.

FULL CYCLE 3.0. Padrões e técnicas avançadas com Git e Github. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 26 mai. 2023.

SONARCLOUD. Clean code in your cloud workflow with {SonarCloud}. 2023. Disponível em: <https://www.sonarsource.com/products/sonarcloud>. Acesso em: 26 mai. 2023.

FULL CYCLE 3.0. API Gateway com Kong e Kubernetes. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 31 mai. 2023.

SPECTRAL. Create a Ruleset. 2023. Disponível em: <https://meta.stoplight.io/docs/spectral/01baf06bdd05a-create-a-ruleset>. Acesso em: 01 jun. 2023.

TERRAFORM. Provision a GKE Cluster (Google Cloud). 2023. Disponível em: <https://developer.hashicorp.com/terraform/tutorials/kubernetes/gke>. Acesso em: 01 jun. 2023.

FULL CYCLE 3.0. GitOps. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 02 jun. 2023.

ARGO CD. Getting Started. 2023. Disponível em: <https://argo-cd.readthedocs.io/en/stable/getting_started>. Acesso em: 02 jun. 2023.

FULL CYCLE 3.0. Terraform. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 04 jun. 2023.

KUBERNETES DOCUMENTATION. Declarative Management of Kubernetes Objects Using Kustomize. 2023. Disponível em: <https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization>. Acesso em: 04 jun. 2023.

KONG DOCS. Custom Resources. 2023. Disponível em: <https://docs.konghq.com/kubernetes-ingress-controller/latest/concepts/custom-resources/>. Acesso em: 09 jun. 2023.

KONG DOCS. Rate Limiting. 2023. Disponível em: <https://docs.konghq.com/hub/kong-inc/rate-limiting/>. Acesso em: 09 jun. 2023.

KONG DOCS. Kong Ingress Controller annotations. 2023. Disponível em: <https://docs.konghq.com/kubernetes-ingress-controller/latest/references/annotations/>. Acesso em: 09 jun. 2023.

POSTMAN. Test script examples. 2023. Disponível em: <https://learning.postman.com/docs/writing-scripts/script-references/test-examples/#asserting-array-properties>. Acesso em: 31 mai. 2023.
