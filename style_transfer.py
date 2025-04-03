import torch
import torch.nn as nn
import torch.optim as optim
import torchvision.models as models
import torchvision.transforms as transforms

from PIL import Image
import sys
import os

device = 'cpu' if torch.cuda.is_available() else 'cpu'

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
        self.model = models.vgg19(pretrained=True).features[:29]

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
    model = VGG().to(device).eval()
    image = load_image(style_image_path)
    features = model(image)
    torch.save(features, out_tensor_path)
    print(f"✅ Признаки стиля сохранены в {out_tensor_path}")

# Применение стиля по признакам
def apply_style(content_path, style_tensor_path, output_path):
    model = VGG().to(device).eval()
    content = load_image(content_path)
    style_feat = torch.load(style_tensor_path, weights_only=False)
    generated = content.clone().requires_grad_(True)

    optimizer = optim.Adam([generated], lr=0.004)
    epochs = 100

    for i in range(epochs):
        gen_feat = model(generated)
        cont_feat = model(content)

        loss = calculate_total_loss(gen_feat, cont_feat, style_feat)

        optimizer.zero_grad()
        loss.backward()
        optimizer.step()

        if i % 100 == 0:
            print(f"[{i}/{epochs}] Loss: {loss.item():.4f}")

    save_output(generated, output_path)
    print(f"✅ Стилизация завершена. Сохранено в {output_path}")

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
