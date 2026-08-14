import http from 'k6/http';
import { check, sleep } from 'k6';

// 压测配置：模拟 100 个虚拟用户，持续压测 30 秒
export const options = {
    vus: 100,
    duration: '30s',
};

export default function () {
    const url = 'https://你的域名/v1/chat/completions';

    const payload = JSON.stringify({
        model: 'gpt-3.5-turbo',
        messages: [{ role: 'user', content: 'Say "Hello"' }],
        stream: false // 压测吞吐量建议关闭流式
    });

    const params = {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer sk-xxxxxxxxxxxxxxxxxxxxxxxx',
        },
        timeout: '120s'
    };

    // 发起请求
    const res = http.post(url, payload, params);

    // 断言检查
    check(res, {
        'status is 200': (r) => r.status === 200,
        'no timeout': (r) => r.status !== 408,
    });

    // 每次请求后稍微停顿，模拟真实用户
    sleep(1);
}