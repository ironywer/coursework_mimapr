import torch
import torch.nn as nn
import torch.optim as optim
import torchvision.models as models
import torchvision.transforms as transforms
from torchvision.models import vgg19, VGG19_Weights
from PIL import Image
import sys
import os

device = 'cuda' if torch.cuda.is_available() else 'cpu'

# Загрузка и преобразование изображения
def load_image(path):
    transform = transforms.Compose([
        transforms.Resize((512, 512)),
        transforms.ToTensor()
    ])
    image = Image.open(path).convert("RGB")
    image = transform(image).unsqueeze(0)
    return image.to(device)

# Сохранение изображения
def save_output(tensor, path):
    image = tensor.clone().detach().cpu().squeeze(0)
    image = transforms.ToPILImage()(image)
    image.save(path)

# Модель VGG для извлечения признаков
class VGG(nn.Module):
    def __init__(self):
        super(VGG, self).__init__()
        self.req_features = ['0', '5', '10', '19', '28']
        weights_path = os.environ.get("VGG_WEIGHTS")
        if weights_path and os.path.exists(weights_path):
            print(f"🔍 Загрузка весов VGG19 из {weights_path}")
            base = vgg19(weights=None)
            state = torch.load(weights_path, map_location=device)
            base.load_state_dict(state)
        else:
            if weights_path:
                print(f"⚠️ Файл весов не найден: {weights_path}. Пытаемся скачивать из torchvision")
            base = vgg19(weights=VGG19_Weights.DEFAULT)
        self.model = base.features[:29]


    def forward(self, x):
        features = []
        for layer_num, layer in enumerate(self.model):
            x = layer(x)
            if str(layer_num) in self.req_features:
                features.append(x)
        return features

# Потери
def content_loss(g, o):
    return torch.mean((g - o) ** 2)

def style_loss(g, s):
    b, c, h, w = g.shape
    G = torch.mm(g.view(c, h * w), g.view(c, h * w).t())
    A = torch.mm(s.view(c, h * w), s.view(c, h * w).t())
    return torch.mean((G - A) ** 2)

def calculate_total_loss(gf, cf, sf, alpha=8, beta=70):
    c_loss = 0
    s_loss = 0
    for g, c, s in zip(gf, cf, sf):
        c_loss += content_loss(g, c)
        s_loss += style_loss(g, s)
    return alpha * c_loss + beta * s_loss

# Извлечение признаков стиля
def extract_style(style_image_path, out_tensor_path):
    try:
        model = VGG().to(device).eval()
        image = load_image(style_image_path)
        features = model(image)
        torch.save(features, out_tensor_path)
        print(f"✅ Признаки стиля сохранены в {out_tensor_path}")
    except Exception as e:
        print(f"❌ Ошибка извлечения стиля: {e}", file=sys.stderr)
        if os.path.exists(out_tensor_path):
            try:
                os.remove(out_tensor_path)
                print(f"🗑 Удалён частично сохранённый файл: {out_tensor_path}")
            except Exception as rmErr:
                print(f"⚠️ Не удалось удалить {out_tensor_path}: {rmErr}", file=sys.stderr)
        sys.exit(1)


# Применение стиля по признакам
def apply_style(content_path, style_tensor_path, output_path):
    model = VGG().to(device).eval()
    content = load_image(content_path)
    style_feat = torch.load(style_tensor_path, weights_only=False)
    generated = content.clone().requires_grad_(True)

    optimizer = optim.Adam([generated], lr=0.004)
    epochs = int(os.environ.get('EPOCHS', 100))
    if epochs <= 0:
        print("❌ Неверное значение EPOCHS. Должно быть больше 0.", file=sys.stderr)
        sys.exit(1)
    try:
        for i in range(epochs):
            gen_feat = model(generated)
            cont_feat = model(content)

            loss = calculate_total_loss(gen_feat, cont_feat, style_feat)

            optimizer.zero_grad()
            loss.backward()
            optimizer.step()

            if i % 100 == 0:
                print(f"[{i}/{epochs}] Loss: {loss.item():.4f}")

        # Сохраняем только если всё прошло без exception
        save_output(generated, output_path)
        print(f"✅ Стилизация завершена. Сохранено в {output_path}")

    except Exception as e:
        # Логируем ошибку
        print(f"❌ Ошибка во время стилизации: {e}", file=sys.stderr)
        # Удаляем потенциально частично записанный файл
        if os.path.exists(output_path):
            try:
                os.remove(output_path)
                print(f"🗑 Удалён неполный файл: {output_path}")
            except OSError as rmErr:
                print(f"⚠️ Не удалось удалить {output_path}: {rmErr}", file=sys.stderr)
        # Завершаем с ошибкой
        sys.exit(1)
# CLI
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Использование:\n"
              "  extract-style <style.jpg> <style.pt>\n"
              "  stylize <content.jpg> <style.pt> <output.jpg>")
        sys.exit(1)

    command = sys.argv[1]

    if command == "extract-style" and len(sys.argv) == 4:
        extract_style(sys.argv[2], sys.argv[3])

    elif command == "stylize" and len(sys.argv) == 5:
        apply_style(sys.argv[2], sys.argv[3], sys.argv[4])

    else:
        print("❌ Неверные аргументы.")
        sys.exit(1)
