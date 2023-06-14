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

Neste momento, vamos definir uma _suite_ de testes que representam a interação do cliente, ou seja, o que o cliente espera quando interage com a nossa _API_.

Para isso, iremos utilizar o mecanismo de testes do _Postman_:

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

Conjuntamente ao _Newman_, iremos utilizar uma ferramenta de _mocking_ chamada _[Prism](https://docs.stoplight.io/docs/prism/83dbbd75532cf-http-mocking)_. O _Prism_ vai expor a rota baseado na estrutura de contrato da _API_ (i.e., o arquivo _yaml_ em formtato _OpenAPI_). Ele consegue, por exemplo, encontrar a definição para os objetos de _request_ e _response_ a partir de propriedades _example_ dentro dessa estrutura.

Com isso, é possível verificar se o _design_ do contrato, exposto através do _Prism_, está aderente ao que o cliente espera. E a definição de o que o cliente espera é contemplado no teste de contrato que o _Newman_ executa.

Então, se for feita alguma alteração na estrutura do contrato que não reflita o que o cliente espera, o teste de contrato vai acusar o problema através dos testes do _Postman_.

Após subir as alterações para o _GitHub_, é possível verificar que as validações executaram com sucesso:

![Check API Contract - Success](./images/check-api-contract-success.png)

Examinando os _logs_ na _Action_ _Check API Contract_, podemos ver que, após subir o _Prism_ na porta _4010_, inicia-se a execução dos testes de contrato do serviço _driver_. E, mais abaixo, os _logs_ do _Postman_ confirmam que todos os testes passaram.

Dessa forma, garantimos que o contrato está coerente com o que o cliente espera.

### ArgoCD

Após instalar o _ArgoCD_, iremos acessar a interface administrativa e logar, conforme descrito na [documentação](https://argo-cd.readthedocs.io/en/stable/#getting-started).

```
$ mkdir infra/kong-k8s/argo
$ touch infra/kong-k8s/argo/argo.sh
$ vim infra/kong-k8s/argo/argo.sh

#!/bin/bash
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

$ cd infra/kong-k8s/argo
$ ./argo.sh
```

Mas, neste caso, para criar uma nova aplicação no _ArgoCD_, não iremos utilizar a interface do _ArgoCD_, vamos utilizar uma configuração do _ArgoCD_ via _CRDs_ (_Custom Resource Definitions_). Desse modo, assim como o _Kong_, o _ArgoCD_ também conta com o seu próprio conjunto de _CRDs_. Mas por que por utilizar _CRDs_ ao invés de utilizar a _interface_ do _ArgoCD_?

A partir do arquivo de configuração, nós temos uma infraestrutura como código, porque ele representa o estado desejado do _cluster_. E, sempre que trabalhamos com _IaC_, procuramos deixar a definição da nossa infraestrutura em arquivos para que alguma ferramenta possa aplicá-los. Já se fosse utilizada a _interface_ do _ArgoCD_ e as configurações da aplicação se perdessem por algum motivo, como perda da base de dados, por exemplo, não teria como reaplicar essas configurações.

```
$ mkdir infra/argo-apps
$ touch infra/argo-apps/driver.yaml
$ vim infra/argo-apps/driver.yaml

apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: driver
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    path: k8s
    repoURL: https://github.com/maratonafullcyclesidartaoss/fullcycle-maratona-1-codelivery-part-6-driver-share.git
    targetRevision: HEAD
    kustomize:
  destination:
    namespace: driver
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      selfHeal: true
      prune: true
    syncOptions:
      - CreateNamespace=true
```

Mas, antes de aplicar o _Application_ do _ArgoCD_, vamos atualizar o manifesto _driver.yaml_ no diretório _k8s_ e subir para o _GitHub_, porque é esse o diretório que o _ArgoCD_ vai monitorar para aplicar os objetos da aplicação _driver_ no _cluster Kubernetes_:

```
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
          image: driver
          resources:
            requests:
              cpu: "0.005"
              memory: 20Mi
            limits:
              cpu: "0.005"
              memory: 25Mi
          ports:
            - containerPort: 8081

---
apiVersion: v1
kind: Service
metadata:
  name: driver
  annotations:
    ingress.kubernetes.io/service-upstream: "true"
  labels:
    app: driver
    stack: echo
    interface: rest
    language: golang
spec:
  type: LoadBalancer
  selector:
    app: driver
  ports:
    - name: http
      port: 80
      protocol: TCP
      targetPort: 8081
```

E, neste momento, podemos aplicar o _Application_ do _ArgoCD_:

```
$ kubectl apply -f infra/argo-apps/driver.yaml -n argocd
```

Lembrando que a configuração do _ArgoCD_ escuta _CRDs_ dentro do _namespace_ _argocd_. Mas, os objetos resultantes desse _CRD_ vão ser criados no _namespace_ _driver_. Então, uma vez que é criado o _CRD_, o próprio objeto _Application_ está configurado para sincronizar automaticamente o repositório do _GitHub_ com o _cluster Kubernetes_:

![Application driver synced](./images/application-driver-synced.png)

### Testes de Carga

O nosso objetivo, agora, é iniciar com _Load Tests_, com foco na ferramenta do _Kong API Gateway_.

Um _API Gateway_ depende de alguns componentes, principalmente o ecossistema de aplicações. Então, é necessário entender a relação de _performance_ entre o _API Gateway_ e as chamadas dos serviços. É necessário entender, por exemplo, se é a aplicação que está apresentando problemas ou se é o _API Gateway_.

Dessa forma, vamos aplicar uma carga para verificar como se dará o comportamento da aplicação e do próprio _API Gateway_.

Deve-se salientar que, neste caso, serão aplicados _load tests_, não serão aplicados _stress tests_, com o objetivo de derrubar a aplicação. O objetivo, aqui, é analisar como o nosso ecossistema conseguirá ficar estável ou não, variando a carga de requisições, ou seja, como a aplicação se comporta com uma carga um pouco mais alta do que a convencional.

Para isso, iremos utilizar duas ferramentas, que vão nos auxiliar no processo de escrita e execução dos testes.

#### K6

Para criar os testes, vamos utilizar o _[K6](https://k6.io/)_. É uma ferramenta _open source_, que faz parte do ecossistema do _Grafana_. Ela permite que sejam escritos testes em _JavaScript_ para serem executados via linha de comando:

```
$ mkdir infra/load
$ touch infra/load/create_driver_load.js

import { check } from "k6";
import http from "k6/http";

const KONG_CLIENT = "kong";
const KONG_SECRET = "gHeYm7teVX4JEb1Vm8ghEJ7EK06IOw4e";
const USER = "maria";
const PASS = "maria";

export const options = {
  stages: [
    { target: 0, duration: "10s" },
    { target: 50, duration: "60s" },
    { target: 100, duration: "60s" },
    { target: 200, duration: "180s" },
  ],
};

function authenticateUsingKeycloak(clientId, clientSecret, username, pass) {
  const formData = {
    client_id: clientId,
    grant_type: "password",
    username: username,
    password: pass,
    client_secret: clientSecret,
    scope: "openid",
  };
  const headers = { "Content-Type": "application/x-www-form-urlencoded" };
  const response = http.post(
    "http://keycloak.iam/realms/driver/protocol/openid-connect/token",
    formData,
    { headers }
  );
  return response.json();
}

export function setup() {
  return authenticateUsingKeycloak(KONG_CLIENT, KONG_SECRET, USER, PASS);
}

export default function (data) {
  const params = {
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${data.access_token}`, // or `Bearer ${clientAuthResp.access_token}`
    },
  };
  let response = http.get(
    "http://kong-kong-proxy.kong/api/driver/drivers",
    params
  );
  check(response, {
    "is status 200": (r) => r.status === 200,
  });
}
```

Com a constante _options_, são definidos os estágios da carga. Por exemplo, no primeiro estágio, temos uma carga para 0 _virtual users_ por 10 segundos. No estágio seguinte, configuramos uma carga para 50 _virtual users_ durante 60 segundos. Depois, configuramos para que suba para 100 _virtual users_ e que dure por 60 segundos e, por fim, subimos para 200 _virtual users_ uma carga que irá durar por 180 segundos.

Adicionalmente, percebe-se que foram adicionadas funções _JavaScript_ para lidar com autenticação, porque o objetivo é testar a _API_ conjuntamente com o fluxo de _OpenID Connect_, de forma a estarmos o mais próximo possível do comportamento em ambiente de Produção.

Interessante notar que o _K6_ tem um _lifecycle_ de teste, similarmente a algumas linguagens de programação (e.g., _@BeforeClass_, @BeforeAll, @BeforeEach), que prepara o ambiente para o teste. Por exemplo, a função _setup()_, que roda antes do teste, é responsável por buscar o _token_. Já a _default function_ executa o teste de carga.

#### Testkube

O nosso objetivo é rodar os testes em um ambiente escalável, ou seja, dentro de um _cluster_ _Kubernetes_. E, para isso, vamos instalar outra ferramenta que automatiza o processo de execução dos testes: o _Testkube_. Assim, a partir da criação do _script_ do _K6_ e da instalação do _Testkube_, será possível aplicar a carga.

O _Testkube_, a partir de artefatos do _Kubernetes_, ajuda a rodar a carga dentro do _cluster_; ele automatiza o processo de execução de testes, atuando como um orquestrador do _script_ _K6_ que vai rodar.

```
$ mkdir infra/load/infra
$ touch infra/load/infra/install.sh

#!/bin/bash
kubectl testkube install

$ cd infra/load/infra/
$ sudo chmod -R 777 .
$ ./install.sh
```

A partir da instalação, é criado um novo _namespace_, chamado _testkube_, contendo diversos objetos que vão provisionar a infraestrutura para a execução dos testes:

```
$ kubectl get all -n testkube

NAME                                                        READY   STATUS    RESTARTS        AGE
pod/testkube-api-server-5b5db9b745-9jpjc                    1/1     Running   1 (5m11s ago)   5m52s
pod/testkube-dashboard-65b84ff9d-qqwbq                      1/1     Running   0               5m52s
pod/testkube-minio-testkube-bd549c85d-mhdkz                 1/1     Running   0               5m52s
pod/testkube-mongodb-d78699775-4lbg2                        1/1     Running   0               5m52s
pod/testkube-nats-0                                         3/3     Running   0               5m52s
pod/testkube-nats-box-5b555bc9c4-bhrdt                      1/1     Running   0               5m52s
pod/testkube-operator-controller-manager-76b79584b4-bzwwx   2/2     Running   0               5m52s

NAME                                                           TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)                                                 AGE
service/testkube-api-server                                    ClusterIP   10.7.241.183   <none>        8088/TCP                                                5m53s
service/testkube-dashboard                                     ClusterIP   10.7.245.166   <none>        8080/TCP                                                5m53s
service/testkube-minio-service-testkube                        ClusterIP   10.7.250.173   <none>        9000/TCP,9090/TCP,9443/TCP                              5m53s
service/testkube-mongodb                                       ClusterIP   10.7.243.157   <none>        27017/TCP                                               5m53s
service/testkube-nats                                          ClusterIP   None           <none>        4222/TCP,6222/TCP,8222/TCP,7777/TCP,7422/TCP,7522/TCP   5m53s
service/testkube-operator-controller-manager-metrics-service   ClusterIP   10.7.250.233   <none>        8443/TCP                                                5m53s
service/testkube-operator-webhook-service                      ClusterIP   10.7.243.202   <none>        443/TCP                                                 5m53s

NAME                                                   READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/testkube-api-server                    1/1     1            1           5m53s
deployment.apps/testkube-dashboard                     1/1     1            1           5m53s
deployment.apps/testkube-minio-testkube                1/1     1            1           5m53s
deployment.apps/testkube-mongodb                       1/1     1            1           5m53s
deployment.apps/testkube-nats-box                      1/1     1            1           5m53s
deployment.apps/testkube-operator-controller-manager   1/1     1            1           5m53s

NAME                                                              DESIRED   CURRENT   READY   AGE
replicaset.apps/testkube-api-server-5b5db9b745                    1         1         1       5m54s
replicaset.apps/testkube-dashboard-65b84ff9d                      1         1         1       5m54s
replicaset.apps/testkube-minio-testkube-bd549c85d                 1         1         1       5m54s
replicaset.apps/testkube-mongodb-d78699775                        1         1         1       5m54s
replicaset.apps/testkube-nats-box-5b555bc9c4                      1         1         1       5m54s
replicaset.apps/testkube-operator-controller-manager-76b79584b4   1         1         1       5m54s

NAME                             READY   AGE
statefulset.apps/testkube-nats   1/1     5m54s
```

Dessa forma, a ferramenta já está provisionada para receber alguma carga.

### Preparando o cluster

#### metrics-server

Um objeto _Deployment_ tem, por padrão, definido o número de réplicas como apenas um. Só que esse não é o comportamento esperado quando se faz teste de carga: não se espera que o número de instâncias fique em apenas um.

O comportamento desejado é que o _Kubernetes_, com base em algumas métricas do _Deployment_, seja capaz de instanciar novas réplicas da aplicação, fazendo com que ela fique com mais instâncias e consiga receber maior carga. Por exemplo: ao bater uma métrica de 70% de processamento, o _Kubernetes_ já começa a subir novas instâncias, de forma a escalar horizontalmente. Esse processo também é conhecido como _autoscaling_ e é uma das funcionalidades mais conhecidas do _Kubernetes_.

O nosso objetivo aqui, então, é, primeiramente, preparar o _cluster_ para ele que ele consiga executar o processo de _autoscaling_.

A primeira coisa que devemos fazer é verificar se o _metrics-server_ está instalado no _cluster_. Normalmente, o _cloud provider_ já habilita esse recurso por padrão.

```
$ kubectl get po -n kube-system

NAME                                                             READY   STATUS    RESTARTS   AGE
event-exporter-gke-755c4b4d97-4xzql                              2/2     Running   0          5h43m
fluentbit-gke-bbwmb                                              2/2     Running   0          5h38m
fluentbit-gke-jqr5t                                              2/2     Running   0          5h37m
fluentbit-gke-qrhnx                                              2/2     Running   0          5h38m
gke-metrics-agent-28jdx                                          2/2     Running   0          5h37m
gke-metrics-agent-5p4sm                                          2/2     Running   0          5h38m
gke-metrics-agent-tntnq                                          2/2     Running   0          5h38m
konnectivity-agent-65c88cbd8d-8f9zq                              1/1     Running   0          5h43m
konnectivity-agent-65c88cbd8d-dqp4s                              1/1     Running   0          5h37m
konnectivity-agent-65c88cbd8d-nrlmd                              1/1     Running   0          5h37m
konnectivity-agent-autoscaler-7dc78c8c9-7ldrf                    1/1     Running   0          5h43m
kube-dns-5b5dfcd97b-n5m4k                                        4/4     Running   0          5h43m
kube-dns-5b5dfcd97b-xq6r5                                        4/4     Running   0          5h43m
kube-dns-autoscaler-5f56f8997c-b7bdn                             1/1     Running   0          5h43m
kube-proxy-gke-maratona-fullcyc-maratona-fullcyc-0b912d49-mh12   1/1     Running   0          5h38m
kube-proxy-gke-maratona-fullcyc-maratona-fullcyc-dd923b2a-xv07   1/1     Running   0          5h37m
kube-proxy-gke-maratona-fullcyc-maratona-fullcyc-f4d99fd0-fc4v   1/1     Running   0          5h38m
l7-default-backend-676d84669b-t9fv9                              1/1     Running   0          5h43m
metrics-server-v0.5.2-67864775dc-tltmv                           2/2     Running   0          5h43m
pdcsi-node-hhrhh                                                 2/2     Running   0          5h37m
pdcsi-node-kkv7n                                                 2/2     Running   0          5h38m
pdcsi-node-kmr46                                                 2/2     Running   0          5h38m
```

O _metrics-server_, basicamente, é um recurso do _Kubernetes_ que organiza as métricas dos _PODs_, coletando a utilização de memória e _CPU_ deles para que ele forneça as informações para o _HorizontalPODAutoscaler_ (_HPA_), que é o objeto do _Kubernetes_ que vai fazer o _upscaling_ e _downscaling_ da aplicação. Assim, para o _HPA_ escalar, ele precisa de métricas, que são providas pelo _metrics-server_.

#### HPA

O _HPA_ é um objeto do _Kubernetes_ aonde se define o seguinte comportamento: escalar o _POD_ assim que ele chegar em uma determinada condição. A condição padrão que pode-se utilizar é memória e _CPU_:

```
 $ mkdir infra/kong-k8s/misc/apps/hpa
 $ touch infra/kong-k8s/misc/apps/hpa/driver.yaml
 $ vim infra/kong-k8s/misc/apps/hpa/driver.yaml

apiVersion: autoscaling/v1
kind: HorizontalPodAutoscaler
metadata:
  name: driver-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: driver
  minReplicas: 1
  maxReplicas: 20
  targetCPUUtilizationPercentage: 5

```

Não deve-se esquecer de definir o _Deployment_ para o qual esse _HPA_ está associado.

Neste caso, definimos um número máximo de 20 réplicas. O que isso significa? Significa que, se chegar a atingir o limite de 20 instâncias, com base na carga de processamento de _CPU_, o _Kubernetes_ não vai subir mais instâncias da aplicação.

A propriedade _targetCPUUtilizationPercentage_ indica que, se o _POD_ atingir 70% de utilização de _CPU_, o _Kubernetes_ começa a subir novas instâncias, baseando-se na definição do objeto de _Deployment_, até atingir o limite de 20 instâncias.

```
  $ kubectl apply -f infra/kong-k8s/misc/apps/hpa/driver.yaml -n driver
```

Após aplicar o _HPA_, vamos consultar esse objeto no _cluster_:

```
$ kubectl get horizontalpodautoscaler -n driver

NAME         REFERENCE           TARGETS   MINPODS   MAXPODS   REPLICAS   AGE
driver-hpa   Deployment/driver   0%/5%     1         20        4          2m57s
```

A partir dessa consulta, já sabemos que o _metrics-server_ está funcionando, porque está apresentando os valores na coluna _TARGETS_, o que indica que o _HPA_ está recebendo as métricas.

#### Prometheus

Neste momento, é necessário configurar o _Prometheus_ para fazer o _scraping_ de métricas do _Kong_ de forma a coletar métricas de todos os _namespaces_, usando _ServiceMonitor_.

> O Prometheus Operator inclui um _CRD_ que permite a definição do _ServiceMonitor_. O ServiceMonitor é usado para definir uma aplicação da qual se deseja extrair métricas a partir do _cluster_ do _Kubernetes_; o _controller_ aciona os ServiceMonitors que definimos e cria automaticamente a configuração necessária do Prometheus. Fonte: <https://observability.thomasriley.co.uk/prometheus/configuring-prometheus/using-service-monitors/>

A partir de agora, então, o _Prometheus_ passa a coletar métricas de _API_ para os testes de carga, ao invés de coletar métricas de utilização de _CPU_.

```
$ touch infra/kong-k8s/misc/prometheus/prometheus.yaml
$ vim infra/kong-k8s/misc/prometheus/prometheus.yaml

prometheus:
  prometheusSpec:
    ruleSelector: {}
    ruleNamespaceSelector: {}
    ruleSelectorNilUsesHelmValues: false
    serviceMonitorSelector: {}
    serviceMonitorNamespaceSelector: {}
    serviceMonitorSelectorNilUsesHelmValues: false
    podMonitorSelector: {}
    podMonitorNamespaceSelector: {}
    podMonitorSelectorNilUsesHelmValues: false

$ cd infra/kong-k8s/misc/prometheus/
$ ./prometheus.sh
```

É importante lembrar, também, que deve-se setar essas duas variáveis de _ServiceMonitor_ no momento da instalação do _Kong_ para que o _Prometheus_ esteja habilitado para fazer a coleta de métricas:

    - --set serviceMonitor.enabled=true
    - --set serviceMonitor.labels.release=promstack

#### Ingress

Neste momento, precisamos adicionar os _plugins_ do _Prometheus_ e de _rate-limit_ no nosso objeto _Ingress_:

```
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: driver-api
  annotations:
    kubernetes.io/ingress.class: "kong"
    konghq.com/override: do-not-preserve-host
    konghq.com/plugins: oidc-driver,rl-by-header,prometheus-driver
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
```

Apenas observando que foi configurado um _rate-limit_ bastante alto de 10.000 requisições por segundo para não ser barrado por _rate-limit_ nos testes de carga.

Dessa forma, a nossa _API_ vai conter os _plugins_ de: - _OpenID Connect_ para validar autenticação; - _rate-limit_ por _header_, isto é, por usuário; - Métricas.

```
$ kubectl apply -f infra/kong-k8s/misc/apis/driver-api.yaml -n driver
```

Com isso, o _cluster_ está preparado para receber carga.

### Aplicando cargas

Antes de aplicar as cargas de testes, é importante verificarmos se o _ServiceMonitor_ foi habilitado na instalação do _Kong_:

```
$ kubectl get servicemonitor -n kong

No resources found in kong namespace.
```

Caso o _ServiceMonitor_ não esteja habilitado, não serão coletadas métricas para a aplicação.

Por que isso pode acontecer? Dependendo da ordem em que foi instalado o _Prometheus_, pode ser que, no momento em que foi instalado o _Kong_, o _ServiceMonitor_ não tenha sido instalado porque o _Prometheus_ foi instalado depois.

Para resolver isso, basta rodar a instalação do _Kong_ novamente.

```
$ helm delete kong -n kong
$ cd infra/kong-k8s/kong/
$ ./kong.sh

$ kubectl get servicemonitor -n kong

NAME        AGE
kong-kong   12s
```

Neste momento, vamos fazer um _port-forward_ para o _Grafana_:

```
$ kubectl port-forward svc/prometheus-stack-grafana 3000:80 -n monitoring
```

Ao acessar _localhost:3000_, é aberto a tela de _login_:

![Tela de login do Grafana](./images/tela-login-grafana.png)

Para logar, deve-se informar: _Username: admin_, _Password: prom-operator_. É aberto a tela inicial do Grafana:

![Tela inicial do Grafana](./images/tela-inicial-grafana.png)

No nosso caso, é necessário importar _dashboards_ para o _Kong_. Então, vamos na aba lateral esquerda / _Dashboards / botão New / Import_.

![Tela de import do Grafana](./images/tela-import-grafana.png)

O _Kong_ possui um _dashboard_ pronto do _Grafana_. Se formos procurar na _Internet_ pelo _Kong_ oficial do _dashboard_ _Grafana_, vamos encontrar um com o _ID_ 7424.

Então, vamos carregar pelo ID 7424 na tela de _Import dashboard_ do _Grafana_, clicando em _Load_. Após, selecionamos _Prometheus_ e fazemos o _import_ do _dashboard_ do _Kong_.

![Tela inicial do dashboard Kong](./images/tela-inicial-do-dashboard-kong.png)

Por enquanto, não tem nada relacionado ao teste de carga em si, porque os testes não foram executados ainda, mas o _dashboard_ está pronto para ser análise.

Agora, como vamos fazer a criação dos testes? A partir de um _script_:

```
$ touch infra/load/infra/load.sh
$ vim infra/load/infra/load.sh

#!/bin/bash
kubectl testkube create test --file ../create_driver_load.js --type k6/script --name create-driver-load
kubectl testkube run test create-driver-load -f

$ cd infra/load/infra/
$ sudo chmod -R 777 .
$ ./load
```

O que esse _script_ faz é criar o teste de carga no _cluster_ e rodá-lo em seguida. Caso seja necessário removê-lo do _cluster_, basta rodar:

```
$ kubectl testkube delete test create-driver-load
```

Após aplicar o _script_, vamos verificar o que foi criado no _namespace_ do _testkube_:

```
$ kubectl get po -n testkube
NAME                                                    READY   STATUS    RESTARTS        AGE
6487b308d5875516d25437ea-ggdxc                          1/1     Running   0               17s
testkube-api-server-5b5db9b745-9jpjc                    1/1     Running   1 (5h55m ago)   5h56m
testkube-dashboard-65b84ff9d-qqwbq                      1/1     Running   0               5h56m
testkube-minio-testkube-bd549c85d-mhdkz                 1/1     Running   0               5h56m
testkube-mongodb-d78699775-4lbg2                        1/1     Running   0               5h56m
testkube-nats-0                                         3/3     Running   0               5h56m
testkube-nats-box-5b555bc9c4-bhrdt                      1/1     Running   0               5h56m
testkube-operator-controller-manager-76b79584b4-bzwwx   2/2     Running   1 (4h10m ago)   5h56m
```

O primeiro identificador representa o _POD_ de teste que a infraestrutura do _testkube_ provisionou. Então, vamos ver os _logs_ desse _container_:

```
$ kubectl logs 6487b308d5875516d25437ea-ggdxc -f -n testkube

{"type":"line","content":"\nrunning (4m37.8s), 182/200 VUs, 127419 complete and 0 interrupted iterations\ndefault   [  90% ] 182/200 VUs  4m37.7s/5m10.0s\n","time":"2023-06-13T00:11:24.646157633Z"}
{"type":"line","content":"\nrunning (4m38.8s), 182/200 VUs, 127774 complete and 0 interrupted iterations\ndefault   [  90% ] 182/200 VUs  4m38.7s/5m10.0s\n","time":"2023-06-13T00:11:25.645117657Z"}
{"type":"line","content":"\nrunning (4m39.8s), 183/200 VUs, 128369 complete and 0 interrupted iterations\ndefault   [  90% ] 183/200 VUs  4m39.7s/5m10.0s\n","time":"2023-06-13T00:11:26.644992687Z"}
{"type":"line","content":"\nrunning (4m40.8s), 183/200 VUs, 128908 complete and 0 interrupted iterations\ndefault   [  91% ] 183/200 VUs  4m40.7s/5m10.0s\n","time":"2023-06-13T00:11:27.648250793Z"}
{"type":"line","content":"\nrunning (4m41.8s), 184/200 VUs, 129483 complete and 0 interrupted iterations\ndefault   [  91% ] 184/200 VUs  4m41.7s/5m10.0s\n","time":"2023-06-13T00:11:28.644962351Z"}
{"type":"line","content":"\nrunning (4m42.8s), 184/200 VUs, 130012 complete and 0 interrupted iterations\ndefault   [  91% ] 184/200 VUs  4m42.7s/5m10.0s\n","time":"2023-06-13T00:11:29.645647018Z"}
{"type":"line","content":"\nrunning (4m43.8s), 185/200 VUs, 130486 complete and 0 interrupted iterations\ndefault   [  92% ] 185/200 VUs  4m43.7s/5m10.0s\n","time":"2023-06-13T00:11:30.646004657Z"}
{"type":"line","content":"\nrunning (4m44.8s), 185/200 VUs, 130865 complete and 0 interrupted iterations\ndefault   [  92% ] 185/200 VUs  4m44.7s/5m10.0s\n","time":"2023-06-13T00:11:31.646271768Z"}
{"type":"line","content":"\nrunning (4m45.8s), 186/200 VUs, 131194 complete and 0 interrupted iterations\ndefault   [  92% ] 186/200 VUs  4m45.7s/5m10.0s\n","time":"2023-06-13T00:11:32.645000278Z"}
{"type":"line","content":"\nrunning (4m46.8s), 187/200 VUs, 131724 complete and 0 interrupted iterations\ndefault   [  92% ] 187/200 VUs  4m46.7s/5m10.0s\n","time":"2023-06-13T00:11:33.648363023Z"}
{"type":"line","content":"\nrunning (4m47.8s), 187/200 VUs, 132262 complete and 0 interrupted iterations\ndefault   [  93% ] 187/200 VUs  4m47.7s/5m10.0s\n","time":"2023-06-13T00:11:34.646583281Z"}
{"type":"line","content":"\nrunning (4m48.8s), 188/200 VUs, 132715 complete and 0 interrupted iterations\ndefault   [  93% ] 188/200 VUs  4m48.7s/5m10.0s\n","time":"2023-06-13T00:11:35.645123819Z"}
{"type":"line","content":"\nrunning (4m49.8s), 188/200 VUs, 133262 complete and 0 interrupted iterations\ndefault   [  93% ] 188/200 VUs  4m49.7s/5m10.0s\n","time":"2023-06-13T00:11:36.644955827Z"}
{"type":"line","content":"\nrunning (4m50.8s), 189/200 VUs, 133701 complete and 0 interrupted iterations\ndefault   [  94% ] 189/200 VUs  4m50.7s/5m10.0s\n","time":"2023-06-13T00:11:37.645327624Z"}
{"type":"line","content":"\nrunning (4m51.8s), 189/200 VUs, 134210 complete and 0 interrupted iterations\ndefault   [  94% ] 189/200 VUs  4m51.7s/5m10.0s\n","time":"2023-06-13T00:11:38.6463088Z"}
{"type":"line","content":"\nrunning (4m52.8s), 190/200 VUs, 134786 complete and 0 interrupted iterations\ndefault   [  94% ] 190/200 VUs  4m52.7s/5m10.0s\n","time":"2023-06-13T00:11:39.64489324Z"}
{"type":"line","content":"\nrunning (4m53.8s), 190/200 VUs, 135308 complete and 0 interrupted iterations\ndefault   [  95% ] 190/200 VUs  4m53.7s/5m10.0s\n","time":"2023-06-13T00:11:40.644986375Z"}
{"type":"line","content":"\nrunning (4m54.8s), 191/200 VUs, 135683 complete and 0 interrupted iterations\ndefault   [  95% ] 191/200 VUs  4m54.7s/5m10.0s\n","time":"2023-06-13T00:11:41.64611089Z"}
{"type":"line","content":"\nrunning (4m55.8s), 192/200 VUs, 135981 complete and 0 interrupted iterations\ndefault   [  95% ] 192/200 VUs  4m55.7s/5m10.0s\n","time":"2023-06-13T00:11:42.64481428Z"}
{"type":"line","content":"\nrunning (4m56.8s), 192/200 VUs, 136510 complete and 0 interrupted iterations\ndefault   [  96% ] 192/200 VUs  4m56.7s/5m10.0s\n","time":"2023-06-13T00:11:43.64566273Z"}
{"type":"line","content":"\nrunning (4m57.8s), 193/200 VUs, 136998 complete and 0 interrupted iterations\ndefault   [  96% ] 193/200 VUs  4m57.7s/5m10.0s\n","time":"2023-06-13T00:11:44.645977034Z"}
{"type":"line","content":"\nrunning (4m58.8s), 193/200 VUs, 137544 complete and 0 interrupted iterations\ndefault   [  96% ] 193/200 VUs  4m58.7s/5m10.0s\n","time":"2023-06-13T00:11:45.645258667Z"}
{"type":"line","content":"\nrunning (4m59.8s), 194/200 VUs, 138092 complete and 0 interrupted iterations\ndefault   [  97% ] 194/200 VUs  4m59.7s/5m10.0s\n","time":"2023-06-13T00:11:46.644967956Z"}
{"type":"line","content":"\nrunning (5m00.8s), 194/200 VUs, 138555 complete and 0 interrupted iterations\ndefault   [  97% ] 194/200 VUs  5m00.7s/5m10.0s\n","time":"2023-06-13T00:11:47.645995845Z"}
{"type":"line","content":"\nrunning (5m01.8s), 195/200 VUs, 139119 complete and 0 interrupted iterations\ndefault   [  97% ] 195/200 VUs  5m01.7s/5m10.0s\n","time":"2023-06-13T00:11:48.644903189Z"}
{"type":"line","content":"\nrunning (5m02.8s), 195/200 VUs, 139526 complete and 0 interrupted iterations\ndefault   [  98% ] 195/200 VUs  5m02.7s/5m10.0s\n","time":"2023-06-13T00:11:49.644868162Z"}
{"type":"line","content":"\nrunning (5m03.8s), 196/200 VUs, 140041 complete and 0 interrupted iterations\ndefault   [  98% ] 196/200 VUs  5m03.7s/5m10.0s\n","time":"2023-06-13T00:11:50.646503728Z"}
{"type":"line","content":"\nrunning (5m04.8s), 197/200 VUs, 140604 complete and 0 interrupted iterations\ndefault   [  98% ] 197/200 VUs  5m04.7s/5m10.0s\n","time":"2023-06-13T00:11:51.645510077Z"}
{"type":"line","content":"\nrunning (5m05.8s), 197/200 VUs, 140904 complete and 0 interrupted iterations\ndefault   [  99% ] 197/200 VUs  5m05.7s/5m10.0s\n","time":"2023-06-13T00:11:52.644998669Z"}
{"type":"line","content":"\nrunning (5m06.8s), 198/200 VUs, 141442 complete and 0 interrupted iterations\ndefault   [  99% ] 198/200 VUs  5m06.7s/5m10.0s\n","time":"2023-06-13T00:11:53.645009462Z"}
{"type":"line","content":"\nrunning (5m07.8s), 198/200 VUs, 141905 complete and 0 interrupted iterations\ndefault   [  99% ] 198/200 VUs  5m07.7s/5m10.0s\n","time":"2023-06-13T00:11:54.645089249Z"}
{"type":"line","content":"\nrunning (5m08.8s), 199/200 VUs, 142313 complete and 0 interrupted iterations\ndefault   [ 100% ] 199/200 VUs  5m08.7s/5m10.0s\n","time":"2023-06-13T00:11:55.644970033Z"}
{"type":"line","content":"\nrunning (5m09.8s), 199/200 VUs, 142791 complete and 0 interrupted iterations\ndefault   [ 100% ] 199/200 VUs  5m09.7s/5m10.0s\n","time":"2023-06-13T00:11:56.644860668Z"}

```

Vamos verificar o _HPA_:

```
$ watch -n 0.5 kubectl get horizontalpodautoscaler -n driver

NAME         REFERENCE           TARGETS   MINPODS   MAXPODS   REPLICAS   AGE
driver-hpa   Deployment/driver   18%/5%    1         20        20         5h3m
```

Percebe-se que o número de réplicas fica volátil, ou seja, o _Kubernetes_ fica tentando estabilizar para conseguir atender a carga de requisições.

Vamos, agora, para o _Grafana_ verificar as métricas do _Kong_. No primeiro painel, temos as métricas relacionadas ao total de requisições por segundo. Podemos ver o total de _Requests per Second_ (_RPS_), o total de _RPS_ por rota e serviço e o total de _RPS_ por rota, serviço e _status code_.

![Primeiro painel do Grafana - requests por segundo](./images/primeiro-painel-grafana-requests-por-segundo.png)

No primeiro gráfico de total de _RPS_, chegamos próximo a um pico de 1K de _RPS_ nos primeiros 2 minutos, baixando esse número até chegarmos a 0 _RPS_ nos próximos 4 minutos.

Da mesma forma, no segundo gráfico de total de _RPS_ por rota e serviço, a rota, isto é, o serviço de _backend_ atinge um pico próximo a 1K de _RPS_ nos primeiros 2 minutos, baixando até atingir 0 nos 4 minutos seguintes.

No terceiro gráfico de total de _RPS_ por rota, serviço e _status code_, percebemos o mesmo comportamento que o gráfico anterior, além de o _status code_ permanecer como 200 tanto para a rota quanto para o serviço.

Outra coisa importante para analisar-se no teste de carga é o painel de latência.

![Segundo painel do Grafana - latência](./images/segundo-painel-grafana-latencia.png)

O que podemos perceber nesse painel?

Examinando os gráficos da terceira faixa (de cima para baixo), podemos ver como o _backend_ está se comportando. Por exemplo, no gráfico de _Upstream time accross all services_, vemos que o serviço de _backend_ teve um breve pico de lentidão de apenas 1.5 segundo, mas, logo foi baixando até chegar próximo de 500ms, o que revela um bom comportamento - a _API_ de _backend_ está se comportando dentro do esperado.

Já na primeira faixa de gráficos, é possível analisar a _performance_ do _proxy_ do _Kong_. Se formos analisar o tempo dos _plugins_ de _rate limit_, métricas e _OpenID Connect_, podemos perceber que não está muito baixo: ele atinge um pico de até 20ms e vai diminuindo até ficar próximo de 5ms. Mas, considerando-se que estamos em um ambiente _cloud_, onde o _cluster_ é formado por 3 máquinas que estão distribuídas entre 3 zonas de disponibilidade distintas, pode-se dizer que esse é um comportamento razoável.

Na segunda faixa de gráficos, é possível analisar a latência do _backend_ mais o _Kong_. Percebe-se um comportamento muito similar à terceira faixa, onde o tempo vai baixando, ao invés de subir, o que mostra que não estão havendo problemas de _performance_ e o _backend_ está reagindo bem ao _autoscaling_.

De maneira geral, os números do _Kong_ e do _backend_ parecem satisfatórios.

O _K6_, no final, exibe um resumo dos testes:

```
✓ is status 200

     █ setup

     checks.........................: 100.00% ✓ 134736     ✗ 0
     data_received..................: 66 MB   211 kB/s
     data_sent......................: 194 MB  626 kB/s
     http_req_blocked...............: avg=7.13µs   min=1.08µs  med=2.02µs   max=188.58ms p(90)=3.9µs    p(95)=6.86µs
     http_req_connecting............: avg=1.78µs   min=0s      med=0s       max=32.59ms  p(90)=0s       p(95)=0s
     http_req_duration..............: avg=244ms    min=1.56ms  med=220.75ms max=1.47s    p(90)=425.56ms p(95)=560.02ms
       { expected_response:true }...: avg=244ms    min=1.56ms  med=220.75ms max=1.47s    p(90)=425.56ms p(95)=560.02ms
     http_req_failed................: 0.00%   ✓ 0          ✗ 134737
     http_req_receiving.............: avg=86.62µs  min=21.46µs med=43.9µs   max=374.86ms p(90)=81.22µs  p(95)=101.41µs
     http_req_sending...............: avg=40.49µs  min=7.42µs  med=13.11µs  max=161.02ms p(90)=27.13µs  p(95)=33.33µs
     http_req_tls_handshaking.......: avg=0s       min=0s      med=0s       max=0s       p(90)=0s       p(95)=0s
     http_req_waiting...............: avg=243.87ms min=1.51ms  med=220.65ms max=1.47s    p(90)=425.24ms p(95)=559.9ms
     http_reqs......................: 134737  433.957953/s
     iteration_duration.............: avg=244.18ms min=1.7ms   med=220.92ms max=1.47s    p(90)=425.74ms p(95)=560.25ms
     iterations.....................: 134736  433.954732/s
     vus............................: 199     min=0        max=199
     vus_max........................: 200     min=200      max=200
```

Aqui, podemos ver alguns números interessantes. Por exemplo, a média de duração das iterações ficou em 244.18ms e o percentil 95 ficou em 560.25ms, ou seja, não variou muito. Isso é um bom sinal, porque mostra que o _backend_ reagiu bem para escalar.

Com isso, chegamos a uma validação bastante razoável. A partir das métricas providas pelo _Kong_ e usando o _K6_ e o _Testkube_, é possível rodar um teste de carga bem razoável para conseguirmos analisar a _performance_ do _Kong_.

Chegamos à conclusão que o _Kong_ vem performando de maneira razoável, lembrando que chegamos a um pico de quase 1.000 requisições por segundo e levando-se em consideração também toda a infraestrutura que está sendo utilizada na nuvem para o _Kong_ rodar.

A avaliação final é de que o _Kong_ performa de maneira aceitável em relação à aplicação, ou seja, o _API Gateway_ não adicionou latências consideráveis na requisição, inclusive com autenticação.

### Kong em ambientes Produtivos

Neste momento, vamos instalar algumas ferramentas e comportamentos no _Kong_ para nos aproximarmos ainda mais de um ambiente de Produção. Nesse sentido, vamos adicionar mais um ponto que vai nos amparar em entender o comportamento da aplicação: a coleta de _logs_. Sendo assim, além do monitoramento pelo _Prometheus_, vamos adicionar uma _stack_ de coleta de _logs_.

Então, a primeira coisa que iremos fazer é deletar a aplicação no _ArgoCD_.

![Deletando aplicação do ArgoCD](./images/deletando-aplicacao-argocd.png)

E recriar novamente:

```
$ kubectl apply -f infra/argo-apps/driver.yaml -n argocd
```

Neste momento, vamos instalar a parte da infraestrutura para fazer a coleta de _logs_. Por quê? Porque o comportamento desejado é que o _Kong_ produza _logs_ e, para isso, é necessário uma _stack_ de coleta de _logs_.

Neste caso, iremos utilizar um conjunto de ferramentas conhecido como _EFK_ - _Elasticsearch, Fluentd, Kibana_.

E como funciona o _EFK_? O _Fluentd_ é o responsável por coletar os logs e enviar para o _Elasticsearch_. Já o _Kibana_ é responsável por apresentar as informações baseado nos dados do _Elasticsearch_. Ou seja, o _Elasticsearch_ funciona como um _data store_, o _Fluentd_ como um coletor e representa a parte de visualização.

#### Elasticsearch

Neste momento, vamos iniciar a instalação do _Elasticsearch_.

```
$ mkdir infra/kong-k8s/efk
$ mkdir infra/kong-k8s/efk/elastic
$ touch infra/kong-k8s/efk/elastic/elastic.sh
$ vim infra/kong-k8s/efk/elastic/elastic.sh

#!/bin/bash
kubectl create namespace logs
helm repo add elastic https://helm.elastic.co
helm install elasticsearch elastic/elasticsearch \
  --version=7.17.1 \
  --namespace=logs \
  -f elastic-values.yaml

$ touch infra/kong-k8s/efk/elastic/elastic-values.yaml
$ vim infra/kong-k8s/efk/elastic/elastic-values.yaml

# Permit co-located instances for solitary minikube virtual machines.
antiAffinity: "soft"

# Shrink default JVM heap.
esJavaOpts: "-Xmx128m -Xms128m"

# Allocate smaller chunks of memory per pod.
resources:
  requests:
    cpu: "100m"
    memory: "512M"
  limits:
    cpu: "1000m"
    memory: "512M"

# Request smaller persistent volumes.
volumeClaimTemplate:
  accessModes: ["ReadWriteOnce"]
  storageClassName: "data"
  resources:
    requests:
      storage: 100M


$ cd infra/kong-k8s/efk/elastic/
$ sudo chmod -R 777 .
$ ./elastic.sh

namespace/logs created
"elastic" already exists with the same configuration, skipping
NAME: elasticsearch
LAST DEPLOYED: Tue Jun 13 12:00:28 2023
NAMESPACE: logs
STATUS: deployed
REVISION: 1
NOTES:
1. Watch all cluster members come up.
  $ kubectl get pods --namespace=logs -l app=elasticsearch-master -w2. Test cluster health using Helm test.
  $ helm --namespace=logs test elasticsearch
```

Agora, vamos consultar os objetos no novo _namespace_ de _logs_:

```
$ kubectl get all -n logs

NAME                         READY   STATUS    RESTARTS   AGE
pod/elasticsearch-master-0   0/1     Pending   0          2m8s
pod/elasticsearch-master-1   0/1     Pending   0          2m8s
pod/elasticsearch-master-2   0/1     Pending   0          2m8s

NAME                                    TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)             AGE
service/elasticsearch-master            ClusterIP   10.7.246.183   <none>        9200/TCP,9300/TCP   2m8s
service/elasticsearch-master-headless   ClusterIP   None           <none>        9200/TCP,9300/TCP   2m8s

NAME                                    READY   AGE
statefulset.apps/elasticsearch-master   0/3     2m9s
```

E vemos que os _PODs_ permanecem no _status_ de _Pending_. Por quê?

Ao descrever um dos _PODs_:

```
$ kubectl describe po elasticsearch-master-0 -n logs
```

, é possível ver que ele está associado a um _PersistentVolumeClaim_ (_PVC_):

```
Type: PersistentVolumeClaim (a reference to a PersistentVolumeClaim in the same namespace)
ClaimName: elasticsearch-master-elasticsearch-master-0
```

Só que, ao listar os _PVCs_:

```
$ kubectl get pvc -n logs

NAME                                          STATUS    VOLUME   CAPACITY   ACCESS MODES   STORAGECLASS   AGE
elasticsearch-master-elasticsearch-master-0   Pending                                      data           52m
elasticsearch-master-elasticsearch-master-1   Pending                                      data           52m
elasticsearch-master-elasticsearch-master-2   Pending                                      data           52m
```

, vemos que _elasticsearch-master-elasticsearch-master-0_ não está associado (_STATUS_ _Bound_) com nenhum _PersistentVolume_ (_PV_) na coluna _VOLUME_. Então, é por esse motivo que o _POD_ permanece no _status_ de _Pending_: porque ele está associado a um _PVC_ que está aguardando ser associado a um _PV_.

Então, como resolver isso? Podemos resolver isso simplesmente aplicando um novo objeto do tipo _StorageClass_. A partir da criação de um novo _StorageClass_, são alocados, automaticamente, pelo _Kubernets_, os _PVs_ para os respectivos _PVCs_.

```
$ touch infra/kong-k8s/efk/elastic/storage-class.yaml
$ vim infra/kong-k8s/efk/elastic/storage-class.yaml

kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: data
  namespace: logs
provisioner: pd.csi.storage.gke.io
parameters:
  type: pd-standard
  replication-type: none
volumeBindingMode: WaitForFirstConsumer
allowedTopologies:
  - matchLabelExpressions:
      - key: topology.gke.io/zone
        values:
          - us-central1-a
          - us-central1-b
          - us-central1-c


$ kubectl apply -f infra/kong-k8s/efk/elastic/storage-class.yaml -n logs
```

Lembrando que um objeto _StorageClass_ provê uma espécie de perfil para os objetos de armazenamento:

> Um StorageClass fornece uma maneira para os administradores descreverem as "classes" de armazenamento que são oferecidas. Classes diferentes podem ser mapeadas para níveis de qualidade de serviço, para políticas de _backup_ ou para políticas arbitrárias determinadas pelos administradores do cluster (...) Esse conceito às vezes é chamado de "perfis" em outros sistemas de armazenamento. Fonte: <https://kubernetes.io/docs/concepts/storage/storage-classes/>.

Além de prover novos _PVs_, associando-os de forma automática (_STATUS Bound_) aos _PVCs_, definir um novo objeto de _StorageClass_ resolve um potencial problema de conflito de afinidade com o nó do volume. Como assim? O _POD_ pode estar em uma zona de disponibilidade diferente da zona de disponibilidade do volume; dessa forma, o _POD_ não consegue conectar-se ao volume, gerando um erro de _volume node affinity conflict_.

O nosso _cluster_, por exemplo, contém 3 nós distribuídos em 3 zonas de disponibilidade distintas. E como descobrir as zonas de disponibilidade de cada um dos nós? Para isso, precisamos entrar em cada um dos nós e procurar pelo _label_ _failure-domain.beta.kubernetes.io/zone_ ou _topology.kubernetes.io/zone_:

```
$ kubectl get nodes

NAME                                                  STATUS   ROLES    AGE   VERSION
gke-maratona-fullcyc-maratona-fullcyc-0b912d49-mh12   Ready    <none>   26h   v1.25.8-gke.500
gke-maratona-fullcyc-maratona-fullcyc-dd923b2a-xv07   Ready    <none>   26h   v1.25.8-gke.500
gke-maratona-fullcyc-maratona-fullcyc-f4d99fd0-fc4v   Ready    <none>   26h   v1.25.8-gke.500


$ kubectl describe node gke-maratona-fullcyc-maratona-fullcyc-0b912d49-mh12

Name:               gke-maratona-fullcyc-maratona-fullcyc-0b912d49-mh12
Roles:              <none>
Labels:             beta.kubernetes.io/arch=amd64
                    beta.kubernetes.io/instance-type=n1-standard-1
                    beta.kubernetes.io/os=linux
                    cloud.google.com/gke-boot-disk=pd-balanced
                    cloud.google.com/gke-container-runtime=containerd
                    cloud.google.com/gke-cpu-scaling-level=1
                    cloud.google.com/gke-logging-variant=DEFAULT
                    cloud.google.com/gke-max-pods-per-node=110
                    cloud.google.com/gke-nodepool=maratona-fullcycle-388513-gke
                    cloud.google.com/gke-os-distribution=cos
                    cloud.google.com/gke-provisioning=standard
                    cloud.google.com/gke-stack-type=IPV4
                    cloud.google.com/machine-family=n1
                    cloud.google.com/private-node=false
                    env=maratona-fullcycle-388513
                    failure-domain.beta.kubernetes.io/region=us-central1
                    failure-domain.beta.kubernetes.io/zone=us-central1-a
                    kubernetes.io/arch=amd64
                    kubernetes.io/hostname=gke-maratona-fullcyc-maratona-fullcyc-0b912d49-mh12
                    kubernetes.io/os=linux
                    node.kubernetes.io/instance-type=n1-standard-1
                    topology.gke.io/zone=us-central1-a
                    topology.kubernetes.io/region=us-central1
                    topology.kubernetes.io/zone=us-central1-a
```

#### Fluentd

O _Fluentd_ realiza a coleta de _logs_ via _TCP_. Já o _Kong_ envia os _logs_ em um formato _JSON_. Após fazer a coleta, os _logs_ são enviados para o _Elasticsearch_.

```
$ mkdir infra/kong-k8s/efk/fluentd
$ touch infra/kong-k8s/efk/fluentd/fluentd.sh
$ vim infra/kong-k8s/efk/fluentd/fluentd.sh

#!/bin/bash
helm repo add fluent https://fluent.github.io/helm-charts
helm install fluentd fluent/fluentd --namespace=logs -f fluentd-values.yaml

$ touch infra/kong-k8s/efk/fluentd/fluentd-values.yaml
$ vim infra/kong-k8s/efk/fluentd/fluentd-values.yaml

nameOverride: ""
fullnameOverride: ""

# DaemonSet, Deployment or StatefulSet
kind: "DaemonSet"

# # Only applicable for Deployment or StatefulSet
# replicaCount: 1

image:
  repository: "fluent/fluentd-kubernetes-daemonset"
  pullPolicy: "IfNotPresent"
  tag: ""

## Optional array of imagePullSecrets containing private registry credentials
## Ref: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/
imagePullSecrets: []

serviceAccount:
  create: true
  annotations: {}
  name: null

rbac:
  create: true

# Configure podsecuritypolicy
# Ref: https://kubernetes.io/docs/concepts/policy/pod-security-policy/
podSecurityPolicy:
  enabled: true
  annotations: {}

## Security Context policies for controller pods
## See https://kubernetes.io/docs/tasks/administer-cluster/sysctl-cluster/ for
## notes on enabling and using sysctls
##
podSecurityContext:
  {}
  # seLinuxOptions:
#   type: "spc_t"

securityContext:
  {}
  # capabilities:
  #   drop:
  #   - ALL
  # readOnlyRootFilesystem: true
  # runAsNonRoot: true
# runAsUser: 1000

# Configure the livecycle
# Ref: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/
lifecycle:
  {}
  # preStop:
  #   exec:
#     command: ["/bin/sh", "-c", "sleep 20"]

# Configure the livenessProbe
# Ref: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
livenessProbe:
  httpGet:
    path: /metrics
    port: metrics
  # initialDelaySeconds: 0
  # periodSeconds: 10
  # timeoutSeconds: 1
  # successThreshold: 1
  # failureThreshold: 3

# Configure the readinessProbe
# Ref: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
readinessProbe:
  httpGet:
    path: /metrics
    port: metrics
  # initialDelaySeconds: 0
  # periodSeconds: 10
  # timeoutSeconds: 1
  # successThreshold: 1
  # failureThreshold: 3

resources:
  {}
  # requests:
  #   cpu: 10m
  #   memory: 128Mi
  # limits:
#   memory: 128Mi

## only available if kind is Deployment
autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 100
  targetCPUUtilizationPercentage: 80
  # targetMemoryUtilizationPercentage: 80
  ## see https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/#autoscaling-on-multiple-metrics-and-custom-metrics
  customRules:
    []
    # - type: Pods
    #   pods:
    #     metric:
    #       name: packets-per-second
    #     target:
    #       type: AverageValue
  #       averageValue: 1k
  ## see https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-configurable-scaling-behavior
  # behavior:
  #   scaleDown:
  #     policies:
  #       - type: Pods
  #         value: 4
  #         periodSeconds: 60
  #       - type: Percent
  #         value: 10
  #         periodSeconds: 60

# priorityClassName: "system-node-critical"

nodeSelector: {}

## Node tolerations for server scheduling to nodes with taints
## Ref: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
##
tolerations: []
# - key: null
#   operator: Exists
#   effect: "NoSchedule"

## Affinity and anti-affinity
## Ref: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#affinity-and-anti-affinity
##
affinity: {}

## Annotations to be added to fluentd DaemonSet/Deployment
##
annotations: {}

## Labels to be added to fluentd DaemonSet/Deployment
##
labels: {}

## Annotations to be added to fluentd pods
##
podAnnotations: {}

## Labels to be added to fluentd pods
##
podLabels: {}

## How long (in seconds) a pods needs to be stable before progressing the deployment
##
minReadySeconds:

## How long (in seconds) a pod may take to exit (useful with lifecycle hooks to ensure lb deregistration is done)
##
terminationGracePeriodSeconds:

## Deployment strategy / DaemonSet updateStrategy
##
updateStrategy: {}
#   type: RollingUpdate
#   rollingUpdate:
#     maxUnavailable: 1

## Additional environment variables to set for fluentd pods
env:
  - name: "FLUENTD_CONF"
    value: "../../../etc/fluent/fluent.conf"
    # - name: FLUENT_ELASTICSEARCH_HOST
    #   value: "elasticsearch-master"
    # - name: FLUENT_ELASTICSEARCH_PORT
    #   value: "9200"

envFrom: []

initContainers: []

# volumes:
#   - name: varlog
#     hostPath:
#       path: /var/log
#   - name: varlibdockercontainers
#     hostPath:
#       path: /var/lib/docker/containers
#   - name: etcfluentd-main
#     configMap:
#       name: fluentd-main
#       defaultMode: 0777
#   - name: etcfluentd-config
#     configMap:
#       name: fluentd-config
#       defaultMode: 0777

# volumeMounts:
# - name: varlog
#   mountPath: /var/log
# - name: varlibdockercontainers
#   mountPath: /var/lib/docker/containers
#   readOnly: true
# - name: etcfluentd-main
#   mountPath: /etc/fluent
# - name: etcfluentd-config
#   mountPath: /etc/fluent/config.d/

## Only available if kind is StatefulSet
## Fluentd persistence
##
persistence:
  enabled: false
  storageClass: ""
  accessMode: ReadWriteOnce
  size: 10Gi

## Fluentd service
##
service:
  type: "ClusterIP"
  annotations: {}
  # loadBalancerIP:
  # externalTrafficPolicy: Local
  ports:
    - name: "forwarder"
      protocol: TCP
      containerPort: 24224

## Prometheus Monitoring
##
metrics:
  serviceMonitor:
    enabled: false
    additionalLabels:
      release: prometheus-operator
    namespace: ""
    namespaceSelector: {}
    ## metric relabel configs to apply to samples before ingestion.
    ##
    metricRelabelings: []
    # - sourceLabels: [__name__]
    #   separator: ;
    #   regex: ^fluentd_output_status_buffer_(oldest|newest)_.+
    #   replacement: $1
    #   action: drop
    ## relabel configs to apply to samples after ingestion.
    ##
    relabelings: []
    # - sourceLabels: [__meta_kubernetes_pod_node_name]
    #   separator: ;
    #   regex: ^(.*)$
    #   targetLabel: nodename
    #   replacement: $1
    #   action: replace
    ## Additional serviceMonitor config
    ##
    # jobLabel: fluentd
    # scrapeInterval: 30s
    # scrapeTimeout: 5s
    # honorLabels: true

  prometheusRule:
    enabled: false
    additionalLabels: {}
    namespace: ""
    rules: []
    # - alert: FluentdDown
    #   expr: up{job="fluentd"} == 0
    #   for: 5m
    #   labels:
    #     context: fluentd
    #     severity: warning
    #   annotations:
    #     summary: "Fluentd Down"
    #     description: "{{ $labels.pod }} on {{ $labels.nodename }} is down"
    # - alert: FluentdScrapeMissing
    #   expr: absent(up{job="fluentd"} == 1)
    #   for: 15m
    #   labels:
    #     context: fluentd
    #     severity: warning
    #   annotations:
    #     summary: "Fluentd Scrape Missing"
    #     description: "Fluentd instance has disappeared from Prometheus target discovery"

## Grafana Monitoring Dashboard
##
dashboards:
  enabled: "true"
  namespace: ""
  labels:
    grafana_dashboard: '"1"'

## Fluentd list of plugins to install
##
plugins: []
# - fluent-plugin-out-http

## Add fluentd config files from K8s configMaps
##
configMapConfigs:
  - fluentd-prometheus-conf
# - fluentd-systemd-conf

## Fluentd configurations:
##
fileConfigs:
  01_sources.conf: |-
    ## logs from kong
    <source>
      @type tcp
      tag tcp.events # required
      <parse>
        @type json
      </parse>
      port 24224   # optional. 5170 by default
      bind 0.0.0.0 # optional. 0.0.0.0 by default
      delimiter "\n" # optional. "\n" (newline) by default
    </source>

  02_outputs.conf: |-
    <match **>
      @type elasticsearch
      host "elasticsearch-master"
      port 9200
      path ""
      user elastic
      password changeme
    </match>

$ cd infra/kong-k8s/efk/fluentd
$ sudo chmod -R 777 .
$ ./fluentd.sh

NAME: fluentd
LAST DEPLOYED: Tue Jun 13 18:59:10 2023
NAMESPACE: logs
STATUS: deployed
REVISION: 1
NOTES:
Get Fluentd build information by running these commands:

export POD_NAME=$(kubectl get pods --namespace logs -l "app.kubernetes.io/name=fluentd,app.kubernetes.io/instance=fluentd" -o jsonpath="{.items[0].metadata.name}")
kubectl --namespace logs port-forward $POD_NAME 24231:24231
curl http://127.0.0.1:24231/metrics
```

#### Kibana

Da mesma forma, vamos instalar o _Kibana_:

```
$ mkdir infra/kong-k8s/efk/kibana
$ touch infra/kong-k8s/efk/kibana/elastic.sh
$ vim infra/kong-k8s/efk/kibana/elastic.sh

#!/bin/bash
helm install kibana elastic/kibana \
  --version=7.17.1 \
  --namespace=logs \
  --set service.type=NodePort \
  --set service.nodePort=31000 \
  -f kibana-values.yaml

$ touch infra/kong-k8s/efk/kibana/kibana-values.yaml
$ vim infra/kong-k8s/efk/kibana/kibana-values.yaml

# Allocate smaller chunks of memory per pod.
resources:
  requests:
    cpu: "10m"
    memory: "500Mi"
  limits:
    cpu: "10m"
    memory: "500Mi"


$ cd infra/kong-k8s/efk/kibana/
$ sudo chmod -R 777 .
$ ./kibana.sh

NAME: kibana
LAST DEPLOYED: Tue Jun 13 19:17:18 2023
NAMESPACE: logs
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

Neste momento, vamos consultar o _namespace_ _logs_ para confirmar que todos os objetos da _stack_ _EFK_ foram criados:

```
$ kubectl get po -n logs

NAME                            READY   STATUS    RESTARTS   AGE
elasticsearch-master-0          1/1     Running   0          4h30m
elasticsearch-master-1          1/1     Running   0          4h30m
elasticsearch-master-2          1/1     Running   0          4h30m
fluentd-c2h96                   1/1     Running   0          136m
fluentd-p5qtf                   1/1     Running   0          136m
fluentd-vzrfx                   1/1     Running   15         111m
kibana-kibana-5d7cbd95f-8phww   1/1     Running   0          16m
```

#### Configurando coleta de logs

Neste momento, vamos fazer a configuração para a aplicação enviar para o _Deployment_ da _API_ os _logs_ para a _stack_ de coleta.

A partir de agora, vamos fazer a configuração das _APIs_, assim, os _logs_ que serão enviados vão ser os _logs_ relacionados ao _Kong_. Assim, é o _API Gateway_ que vai mandar os _logs_ da _API_ para a _stack_ de coleta.

Então, todas as requisições que vierem a partir de _/api/driver_, serão enviados os _logs_ para a _stack_ de coleta de _logs_.

Deve-se ressaltar que, para ambiente de Produção, no objeto _KongIngress_, nós podemos fazer uma configuração um pouco mais específica relacionada ao _proxy_ do _Kong_. Sendo assim, em ambientes produtivos, é extremamente recomendável especificar os _timeouts_, tanto de escrita e leitura quanto de conexão. Isso é muito importante para a estabilidade da aplicação:

```
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
```

Neste momento, vamos criar uma nova configuração de _plugin_ para a coleta de _logs_. Note que o _host_ indica aonde está a localizado a _stack_ de coleta de _logs_.

```
$ vim k8s/driver.yaml

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
          image: driver
          resources:
            requests:
              cpu: "0.005"
              memory: 20Mi
            limits:
              cpu: "0.005"
              memory: 25Mi
          ports:
            - containerPort: 8081

---
apiVersion: v1
kind: Service
metadata:
  name: driver
  annotations:
    konghq.com/plugins: prometheus-driver,driver-logs
    ingress.kubernetes.io/service-upstream: "true"
  labels:
    app: driver
    stack: echo
    interface: rest
    language: golang
spec:
  type: LoadBalancer
  selector:
    app: driver
  ports:
    - name: http
      port: 80
      protocol: TCP
      targetPort: 8081

---
apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: driver-logs
config:
  host: fluentd.logs.svc.cluster.local
  port: 24224
  tls: false
plugin: tcp-log
```

Além disso, podemos perceber que a configuração do _log_ precisa ser feita no _Service_ também, adicionado à anotação _konghq.com/plugins_ a configuração da coleta de _logs_, ou seja, _driver-logs_.

Isso deve ser suficiente para fazer funcionar a coleta de _logs_ em nossa aplicação. Agora, é necessário subir essas alterações para o _GitHub_, de forma que o _ArgoCD_ sincronize com o _cluster Kubernetes_.

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
