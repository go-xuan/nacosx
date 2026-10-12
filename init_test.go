package nacosx

import (
	"strings"
	"testing"

	"github.com/nacos-group/nacos-sdk-go/vo"
)

// TestConfigValid 配置有效性校验
func TestConfigValid(t *testing.T) {
	cases := []struct {
		name   string
		config *Config
		valid  bool
	}{
		{"完整配置", &Config{Address: "127.0.0.1:8848", Namespace: "public", Group: "DEFAULT_GROUP"}, true},
		{"缺少地址", &Config{Namespace: "public", Group: "DEFAULT_GROUP"}, false},
		{"缺少命名空间", &Config{Address: "127.0.0.1:8848", Group: "DEFAULT_GROUP"}, false},
		{"空白地址", &Config{Address: " ", Namespace: "public", Group: "DEFAULT_GROUP"}, false},
		{"缺少分组", &Config{Address: "127.0.0.1:8848", Namespace: "public"}, true}, // 分组可选
		{"空白分组", &Config{Address: "127.0.0.1:8848", Namespace: "public", Group: " "}, true},
		{"只有分隔符", &Config{Address: ",", Namespace: "public"}, false}, // 没有可用地址
		{"空白分段", &Config{Address: " , ", Namespace: "public"}, false},
		{"多地址含空段", &Config{Address: "127.0.0.1:8848,", Namespace: "public"}, true},
	}
	for _, c := range cases {
		if got := c.config.Valid(); got != c.valid {
			t.Errorf("%s: Valid() = %v, want %v", c.name, got, c.valid)
		}
	}
}

// TestServerConfigs 服务地址解析
func TestServerConfigs(t *testing.T) {
	configs := (&Config{Address: "127.0.0.1:8848, 127.0.0.1:8849"}).ServerConfigs()
	if len(configs) != 2 {
		t.Fatalf("ServerConfigs() length = %d, want 2", len(configs))
	}
	if configs[0].IpAddr != "127.0.0.1" || configs[0].Port != 8848 {
		t.Errorf("ServerConfigs()[0] = %s:%d, want 127.0.0.1:8848", configs[0].IpAddr, configs[0].Port)
	}
	if configs[1].Port != 8849 {
		t.Errorf("ServerConfigs()[1].Port = %d, want 8849", configs[1].Port)
	}

	// 省略端口时使用默认端口，且空地址不应生成服务配置
	configs = (&Config{Address: "10.0.0.1,,10.0.0.2:8850"}).ServerConfigs()
	if len(configs) != 2 {
		t.Fatalf("ServerConfigs() length = %d, want 2", len(configs))
	}
	if configs[0].Port != 8848 {
		t.Errorf("ServerConfigs()[0].Port = %d, want 8848", configs[0].Port)
	}
	if configs[1].Port != 8850 {
		t.Errorf("ServerConfigs()[1].Port = %d, want 8850", configs[1].Port)
	}

	// ipv6地址补回中括号，端口非法时回退默认端口
	configs = (&Config{Address: "[::1]:8848,10.0.0.1:abc"}).ServerConfigs()
	if len(configs) != 2 {
		t.Fatalf("ServerConfigs() length = %d, want 2", len(configs))
	}
	if configs[0].IpAddr != "[::1]" || configs[0].Port != 8848 {
		t.Errorf("ServerConfigs()[0] = %s:%d, want [::1]:8848", configs[0].IpAddr, configs[0].Port)
	}
	if configs[1].Port != 8848 {
		t.Errorf("ServerConfigs()[1].Port = %d, want 8848", configs[1].Port)
	}
}

// TestServerConfigsScheme 协议与上下文路径落到SDK的Scheme与ContextPath上，与AddressUrl口径一致
func TestServerConfigsScheme(t *testing.T) {
	cases := []struct {
		address     string
		scheme      string
		contextPath string
		ipAddr      string
		port        uint64
	}{
		{"127.0.0.1:8848", "", "", "127.0.0.1", 8848},                        // 空值由SDK兜底为http、/nacos
		{"https://nacos.io:8848", "https", "", "nacos.io", 8848},             // 不能把scheme拼进IpAddr
		{"http://127.0.0.1:8848/nacos", "http", "/nacos", "127.0.0.1", 8848}, // 自带路径时不再重复拼
		{"127.0.0.1:8848/custom/", "", "/custom", "127.0.0.1", 8848},         // 忽略结尾多余的斜杠
		{"http://10.0.0.1", "http", "", "10.0.0.1", 8848},                    // 省略端口
	}
	for _, c := range cases {
		configs := (&Config{Address: c.address}).ServerConfigs()
		if len(configs) != 1 {
			t.Fatalf("ServerConfigs() of %q length = %d, want 1", c.address, len(configs))
		}
		got := configs[0]
		if got.Scheme != c.scheme || got.ContextPath != c.contextPath || got.IpAddr != c.ipAddr || got.Port != c.port {
			t.Errorf("ServerConfigs() of %q = %s://%s:%d%s, want %s://%s:%d%s",
				c.address, got.Scheme, got.IpAddr, got.Port, got.ContextPath, c.scheme, c.ipAddr, c.port, c.contextPath)
		}
	}
}

// TestAddressUrl 访问地址拼接，多地址取首个并补全协议
func TestAddressUrl(t *testing.T) {
	cases := []struct {
		address string
		want    string
	}{
		{"127.0.0.1:8848", "http://127.0.0.1:8848/nacos"},
		{"127.0.0.1:8848,127.0.0.1:8849", "http://127.0.0.1:8848/nacos"},
		{"https://nacos.io:8848/", "https://nacos.io:8848/nacos"},
		{"http://127.0.0.1:8848/nacos", "http://127.0.0.1:8848/nacos"}, // 已带路径时不再重复拼接
		{"127.0.0.1:8848/custom/", "http://127.0.0.1:8848/custom"},
		{"", ""},
	}
	for _, c := range cases {
		if got := (&Config{Address: c.address}).AddressUrl(); got != c.want {
			t.Errorf("AddressUrl() of %q = %q, want %q", c.address, got, c.want)
		}
	}
}

// TestNamespaceId 公共命名空间归一化为空串
func TestNamespaceId(t *testing.T) {
	cases := []struct {
		namespace string
		want      string
	}{
		{"public", ""},
		{"PUBLIC", ""},
		{" public ", ""},
		{"e525eafa-f7d7-4029-83d9-008937f9d468", "e525eafa-f7d7-4029-83d9-008937f9d468"},
		{"", ""},
	}
	for _, c := range cases {
		if got := (&Config{Namespace: c.namespace}).ClientConfig().NamespaceId; got != c.want {
			t.Errorf("ClientConfig().NamespaceId of %q = %q, want %q", c.namespace, got, c.want)
		}
	}
}

// TestGroupFallback 分组未配置时按nacos默认分组兜底
func TestGroupFallback(t *testing.T) {
	if got := (&Client{config: &Config{}}).GetGroup(); got != "DEFAULT_GROUP" {
		t.Errorf("GetGroup() with empty group = %q, want %q", got, "DEFAULT_GROUP")
	}
	if got := (&Client{config: &Config{Group: "GROUP_A"}}).GetGroup(); got != "GROUP_A" {
		t.Errorf("GetGroup() = %q, want %q", got, "GROUP_A")
	}
	// Reader未锚定分组时，构造出的配置参数也不能是空分组，否则SDK会拒绝读写
	if got := (&Reader{DataId: "app.yaml"}).ConfigParam().Group; got != "DEFAULT_GROUP" {
		t.Errorf("ConfigParam().Group of empty group = %q, want %q", got, "DEFAULT_GROUP")
	}
}

// TestNewClient 非法配置应返回错误而非nil客户端
func TestNewClient(t *testing.T) {
	if _, err := NewClient(nil); err == nil {
		t.Error("NewClient(nil) should return an error")
	}
	// 地址/命名空间缺失时提前报错，不依赖SDK的模糊提示
	cases := []struct {
		name   string
		config *Config
	}{
		{"空配置", &Config{}},
		{"缺少地址", &Config{Namespace: "public", Group: "DEFAULT_GROUP", Mode: OnlyConfig}},
		{"缺少命名空间", &Config{Address: "127.0.0.1:8848", Group: "DEFAULT_GROUP", Mode: OnlyConfig}},
	}
	for _, c := range cases {
		if client, err := NewClient(c.config); err == nil || client != nil {
			t.Errorf("NewClient with %s should return nil client and error, got client=%v, err=%v", c.name, client, err)
		}
	}
	// 配置合法但模式非法时，应在初始化任何客户端之前报错
	client, err := NewClient(&Config{Address: "127.0.0.1:8848", Namespace: "public", Group: "DEFAULT_GROUP", Mode: 9})
	if err == nil || client != nil {
		t.Errorf("NewClient with invalid mode should return nil client and error, got client=%v, err=%v", client, err)
	} else if !strings.Contains(err.Error(), "invalid nacos mode") {
		t.Errorf("NewClient with invalid mode error = %q, want it to mention invalid nacos mode", err.Error())
	}
}

// TestReaderGetType 配置类型识别，优先dataId后缀，其次显式类型，均无法识别时回退json
func TestReaderGetType(t *testing.T) {
	cases := []struct {
		reader *Reader
		want   string
	}{
		{NewReader("database.yaml"), "yaml"},
		{NewReader("cache.json"), "json"},
		{NewReader("log.toml"), "toml"},
		{NewReader("application"), "json"},                     // 无后缀时回退json
		{NewReader("app.yml"), "yaml"},                         // yml归一为yaml
		{NewReader("com.example.app"), "json"},                 // 后缀不是已知格式时不算类型
		{&Reader{DataId: "app.yml"}, "yaml"},                   // 直接构造结构体也应识别后缀
		{&Reader{DataId: "app.yaml", Type: "json"}, "yaml"},    // dataId后缀优先于显式类型
		{&Reader{DataId: "app.yaml", Type: "xml"}, "yaml"},     // 显式类型非法也不影响后缀
		{&Reader{DataId: "application", Type: "yaml"}, "yaml"}, // 无后缀时才使用显式类型
		{&Reader{DataId: "application", Type: "xml"}, "json"},  // 显式类型非法时回退json
	}
	for _, c := range cases {
		if got := c.reader.GetType(); got != c.want {
			t.Errorf("GetType() of %s = %s, want %s", c.reader.DataId, got, c.want)
		}
	}
}

// TestReaderLocation 配置位置标识
func TestReaderLocation(t *testing.T) {
	reader := NewReader("database.yaml")
	reader.Anchor("DEFAULT_GROUP")
	if got, want := reader.Location(), "nacos@DEFAULT_GROUP@database.yaml"; got != want {
		t.Errorf("Location() = %s, want %s", got, want)
	}
}

// TestReaderReadArgs Read参数校验
func TestReaderReadArgs(t *testing.T) {
	// 无缓存且客户端未初始化时读取失败
	if err := NewReader("database.yaml").Read(&Config{}); err == nil {
		t.Error("Read before initialize should return an error")
	}
	// 空缓存同样视为未读取（无法反序列化），走初始化校验而不是返回nil
	if err := (&Reader{DataId: "database.yaml", Data: []byte{}}).Read(&Config{}); err == nil {
		t.Error("Read with empty cache before initialize should return an error")
	}
	// 命中缓存时不依赖客户端，但目标值必须是非空指针，否则yaml反序列化会panic
	reader := &Reader{DataId: "cache.json", Data: []byte(`{"level":"debug"}`)}
	if err := reader.Read(nil); err == nil {
		t.Error("Read with nil arg should return an error")
	}
	if err := reader.Read(Config{}); err == nil {
		t.Error("Read with non-pointer arg should return an error")
	}
	if err := reader.Read((*Config)(nil)); err == nil {
		t.Error("Read with typed-nil pointer should return an error")
	}
}

// TestReaderReadCache 命中缓存时也应反序列化到目标结构体
func TestReaderReadCache(t *testing.T) {
	reader := &Reader{DataId: "cache.json", Data: []byte(`{"level":"debug"}`)}
	var config struct {
		Level string `json:"level"`
	}
	if err := reader.Read(&config); err != nil {
		t.Fatalf("Read from cache failed: %v", err)
	}
	if config.Level != "debug" {
		t.Errorf("config.Level = %q, want %q", config.Level, "debug")
	}

	// 缓存内容无法反序列化时应报错，而不是静默返回nil
	reader = &Reader{DataId: "cache.json", Data: []byte(`{"level":`)}
	if err := reader.Read(&config); err == nil {
		t.Error("Read with invalid cache content should return an error")
	}
}

// TestReaderReadReplace 反序列化是整体替换：内容中缺失的字段会被重置为空值
func TestReaderReadReplace(t *testing.T) {
	config := struct {
		Level string `json:"level"`
		Name  string `json:"name"`
	}{Level: "info", Name: "old"}
	reader := &Reader{DataId: "cache.json", Data: []byte(`{"level":"debug"}`)}
	if err := reader.Read(&config); err != nil {
		t.Fatalf("Read from cache failed: %v", err)
	}
	if config.Level != "debug" || config.Name != "" {
		t.Errorf("config = %+v, want {Level:debug Name:}", config)
	}
}

// TestReadConfigArgs ReadConfig参数校验
func TestReadConfigArgs(t *testing.T) {
	client := &Client{config: &Config{}}
	param := vo.ConfigParam{DataId: "database.yaml", Group: "DEFAULT_GROUP"}
	if _, err := client.ReadConfig(nil, param); err == nil {
		t.Error("ReadConfig with nil arg should return an error")
	}
	if _, err := client.ReadConfig(Config{}, param); err == nil {
		t.Error("ReadConfig with non-pointer arg should return an error")
	}
	// typed-nil指针同样要拦截，否则yaml反序列化会panic
	if _, err := client.ReadConfig((*Config)(nil), param); err == nil {
		t.Error("ReadConfig with typed-nil pointer should return an error")
	}
}

// TestListenConfigArgs ListenConfig参数校验
func TestListenConfigArgs(t *testing.T) {
	// 回调为空必须提前报错：SDK不校验回调，配置变更时会在其goroutine里panic
	client := &Client{config: &Config{}}
	param := vo.ConfigParam{DataId: "app.yaml", Group: "DEFAULT_GROUP"}
	if err := client.ListenConfig(param); err == nil {
		t.Error("ListenConfig without onChange callback should return an error")
	}
}
